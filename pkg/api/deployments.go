package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

type createDeploymentRequest struct {
	Application string   `json:"application"`
	Policies    []string `json:"policies,omitempty"`
	DryRun      bool     `json:"dryRun,omitempty"`
}

func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	var req createDeploymentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Application == "" {
		writeError(w, http.StatusBadRequest, "application is required")
		return
	}

	// Check application exists.
	appRec, err := s.store.GetApplication(req.Application)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}

	// Check for existing active deployment (one per app).
	existing, err := s.store.GetDeploymentByApp(req.Application)
	if err == nil && existing != nil {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("application %q already has an active deployment %q (status: %s)",
				req.Application, existing.ID, existing.Status))
		return
	}

	id := generateID()
	rec := &store.DeploymentRecord{
		ID:              id,
		ApplicationName: req.Application,
		Status:          "pending",
		Policies:        req.Policies,
	}

	if err := s.store.CreateDeployment(rec); err != nil {
		handleStoreError(w, err, "deployment")
		return
	}

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: id,
		Action:       "created",
		Details:      mustJSON(map[string]any{"policies": req.Policies, "dryRun": req.DryRun}),
	})

	if req.DryRun {
		// Synchronous dry run — compute and return the plan.
		app := recordToApplication(appRec)
		plan, err := s.computePlan(app, rec)
		if err != nil {
			rec.Status = "failed"
			rec.Error = err.Error()
			s.store.UpdateDeployment(rec)
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		planJSON, _ := json.Marshal(plan)
		rec.Status = "planned"
		rec.Plan = planJSON
		s.store.UpdateDeployment(rec)

		s.store.AddHistory(&store.HistoryRecord{
			DeploymentID: id,
			Action:       "planned",
			Details:      planJSON,
		})

		writeJSON(w, http.StatusOK, rec)
		return
	}

	// Async deploy — return immediately, deploy in background.
	go s.runDeployment(id, appRec)

	writeJSON(w, http.StatusAccepted, rec)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	deployments, err := s.store.ListDeployments()
	if err != nil {
		handleStoreError(w, err, "deployments")
		return
	}
	if deployments == nil {
		deployments = []store.DeploymentRecord{}
	}
	writeJSON(w, http.StatusOK, deployments)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.store.GetDeployment(id)
	if err != nil {
		handleStoreError(w, err, "deployment")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.store.GetDeployment(id)
	if err != nil {
		handleStoreError(w, err, "deployment")
		return
	}

	if d.Status != "ready" && d.Status != "failed" {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("cannot destroy deployment in status %q", d.Status))
		return
	}

	// Async destroy.
	go s.runDestroy(d)

	d.Status = "destroying"
	s.store.UpdateDeployment(d)

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: id,
		Action:       "destroying",
	})

	writeJSON(w, http.StatusAccepted, d)
}

func (s *Server) handleDeploymentPlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.store.GetDeployment(id)
	if err != nil {
		handleStoreError(w, err, "deployment")
		return
	}

	appRec, err := s.store.GetApplication(d.ApplicationName)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}

	app := recordToApplication(appRec)
	plan, err := s.computePlan(app, d)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleDeploymentHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Verify deployment exists.
	if _, err := s.store.GetDeployment(id); err != nil {
		handleStoreError(w, err, "deployment")
		return
	}

	history, err := s.store.GetHistory(id)
	if err != nil {
		handleStoreError(w, err, "history")
		return
	}
	if history == nil {
		history = []store.HistoryRecord{}
	}
	writeJSON(w, http.StatusOK, history)
}

// --- Background workers ---

func (s *Server) runDeployment(deploymentID string, appRec *store.ApplicationRecord) {
	d, err := s.store.GetDeployment(deploymentID)
	if err != nil {
		log.Printf("deployment %s: failed to load: %v", deploymentID, err)
		return
	}

	app := recordToApplication(appRec)

	// Phase 1: Planning.
	d.Status = "planning"
	s.store.UpdateDeployment(d)
	s.store.AddHistory(&store.HistoryRecord{DeploymentID: d.ID, Action: "planning"})

	plan, err := s.computePlan(app, d)
	if err != nil {
		s.failDeployment(d, "planning failed: "+err.Error())
		return
	}

	planJSON, _ := json.Marshal(plan)
	d.Plan = planJSON
	d.Status = "deploying"
	s.store.UpdateDeployment(d)
	s.store.AddHistory(&store.HistoryRecord{DeploymentID: d.ID, Action: "planned", Details: planJSON})

	// Phase 2: Applying.
	state := types.NewState(app.Metadata.Name)
	if d.State != nil {
		json.Unmarshal(d.State, state)
	}

	executor := engine.NewExecutor(s.registry)
	if err := executor.Execute(plan, state); err != nil {
		// Save partial state.
		stateJSON, _ := json.Marshal(state)
		d.State = stateJSON
		s.failDeployment(d, "apply failed: "+err.Error())
		return
	}

	// Phase 3: Complete.
	stateJSON, _ := json.Marshal(state)
	d.State = stateJSON
	d.Status = "ready"
	s.store.UpdateDeployment(d)

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "applied",
		Details:      stateJSON,
	})

	log.Printf("deployment %s: complete", deploymentID)
}

func (s *Server) runDestroy(d *store.DeploymentRecord) {
	var state types.State
	if d.State != nil {
		json.Unmarshal(d.State, &state)
	}

	for name, resource := range state.Resources {
		provider, ok := s.registry.Get(resource.Provider)
		if !ok {
			log.Printf("deployment %s: provider %q not found for resource %q", d.ID, resource.Provider, name)
			continue
		}
		if err := provider.Destroy(resource); err != nil {
			s.failDeployment(d, fmt.Sprintf("destroying %s: %v", name, err))
			return
		}
	}

	d.Status = "destroyed"
	d.State = nil
	s.store.UpdateDeployment(d)

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "destroyed",
	})

	log.Printf("deployment %s: destroyed", d.ID)
}

func (s *Server) computePlan(app *types.Application, d *store.DeploymentRecord) (*engine.Plan, error) {
	var evaluator *policy.Evaluator
	if len(d.Policies) > 0 {
		policyTypes, err := s.loadPolicies(d.Policies)
		if err != nil {
			return nil, fmt.Errorf("loading policies: %w", err)
		}
		evaluator, err = policy.NewEvaluator(policyTypes)
		if err != nil {
			return nil, fmt.Errorf("building evaluator: %w", err)
		}
	}

	var currentState *types.State
	if d.State != nil {
		currentState = &types.State{}
		json.Unmarshal(d.State, currentState)
	}

	planner := engine.NewPlanner(s.registry, evaluator)
	return planner.CreatePlan(app, currentState)
}

func (s *Server) failDeployment(d *store.DeploymentRecord, errMsg string) {
	d.Status = "failed"
	d.Error = errMsg
	s.store.UpdateDeployment(d)
	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "failed",
		Details:      mustJSON(map[string]any{"error": errMsg}),
	})
	log.Printf("deployment %s: %s", d.ID, errMsg)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func generateID() string {
	return fmt.Sprintf("dep-%d", time.Now().UnixNano())
}
