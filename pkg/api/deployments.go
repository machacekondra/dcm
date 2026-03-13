package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dcm-io/dcm/pkg/compliance"
	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/scheduler"
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

	switch d.Status {
	case "ready", "impacted":
		// Trigger async resource destruction.
		go s.runDestroy(d)
		d.Status = "destroying"
		s.store.UpdateDeployment(d)
		s.store.AddHistory(&store.HistoryRecord{
			DeploymentID: id,
			Action:       "destroying",
		})
		writeJSON(w, http.StatusAccepted, d)

	case "destroyed", "failed", "planned":
		// Hard delete the record from the database.
		if err := s.store.DeleteDeployment(id); err != nil {
			handleStoreError(w, err, "deployment")
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusConflict,
			fmt.Sprintf("cannot delete deployment in status %q", d.Status))
	}
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

func (s *Server) handleDeploymentApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.store.GetDeployment(id)
	if err != nil {
		handleStoreError(w, err, "deployment")
		return
	}

	if d.Status != "planned" {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("can only apply a planned deployment, current status is %q", d.Status))
		return
	}

	appRec, err := s.store.GetApplication(d.ApplicationName)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}

	// Kick off async apply using the existing plan.
	go s.runApplyPlanned(d, appRec)

	d.Status = "deploying"
	s.store.UpdateDeployment(d)
	s.store.AddHistory(&store.HistoryRecord{DeploymentID: id, Action: "deploying"})

	writeJSON(w, http.StatusAccepted, d)
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
	s.store.UpdateDeployment(d)
	s.store.AddHistory(&store.HistoryRecord{DeploymentID: d.ID, Action: "planned", Details: planJSON})

	// Phase 1.5: Compliance check.
	if s.compliance != nil && s.compliance.HasPolicies() {
		violations, err := s.checkCompliance(context.Background(), app, plan)
		if err != nil {
			s.failDeployment(d, "compliance check error: "+err.Error())
			return
		}
		if len(violations) > 0 {
			msgs := make([]string, len(violations))
			for i, v := range violations {
				msgs[i] = v.Message
			}
			violationJSON := mustJSON(map[string]any{"violations": msgs})
			s.store.AddHistory(&store.HistoryRecord{
				DeploymentID: d.ID,
				Action:       "compliance_failed",
				Details:      violationJSON,
			})
			s.failDeployment(d, fmt.Sprintf("compliance check failed: %d violation(s)", len(violations)))
			return
		}
		log.Printf("deployment %s: compliance check passed", d.ID)
	}

	// Phase 2: Applying.
	d.Status = "deploying"
	s.store.UpdateDeployment(d)
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

func (s *Server) runApplyPlanned(d *store.DeploymentRecord, appRec *store.ApplicationRecord) {
	// Re-read to get latest state.
	d, err := s.store.GetDeployment(d.ID)
	if err != nil {
		log.Printf("deployment %s: failed to load: %v", d.ID, err)
		return
	}

	app := recordToApplication(appRec)

	var plan engine.Plan
	if d.Plan != nil {
		json.Unmarshal(d.Plan, &plan)
	}

	state := types.NewState(app.Metadata.Name)
	if d.State != nil {
		json.Unmarshal(d.State, state)
	}

	executor := engine.NewExecutor(s.registry)
	if err := executor.Execute(&plan, state); err != nil {
		stateJSON, _ := json.Marshal(state)
		d.State = stateJSON
		s.failDeployment(d, "apply failed: "+err.Error())
		return
	}

	stateJSON, _ := json.Marshal(state)
	d.State = stateJSON
	d.Status = "ready"
	s.store.UpdateDeployment(d)

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "applied",
		Details:      stateJSON,
	})

	log.Printf("deployment %s: applied from plan", d.ID)
}

func (s *Server) runDestroy(d *store.DeploymentRecord) {
	var state types.State
	if d.State != nil {
		json.Unmarshal(d.State, &state)
	}

	for name, resource := range state.Resources {
		// Look up by environment name first, fall back to provider type name.
		lookupName := resource.Provider
		if resource.Environment != "" {
			lookupName = resource.Environment
		}
		provider, ok := s.registry.Get(lookupName)
		if !ok {
			log.Printf("deployment %s: provider %q not found for resource %q", d.ID, lookupName, name)
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

	// Load all policies — they self-select via match criteria.
	allPolicies, err := s.loadAllPolicies()
	if err != nil {
		return nil, fmt.Errorf("loading policies: %w", err)
	}
	if len(allPolicies) > 0 {
		evaluator, err = policy.NewEvaluator(allPolicies)
		if err != nil {
			return nil, fmt.Errorf("building evaluator: %w", err)
		}
	}

	var currentState *types.State
	if d.State != nil {
		currentState = &types.State{}
		json.Unmarshal(d.State, currentState)
	}

	sched, err := scheduler.NewScheduler(s.registry, evaluator)
	if err != nil {
		return nil, fmt.Errorf("creating scheduler: %w", err)
	}

	planner := engine.NewPlannerWithScheduler(sched)
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

// markDeploymentsImpacted finds all ready deployments on the given environment
// and marks them as "impacted" so the user can manually rehydrate.
func (s *Server) markDeploymentsImpacted(envName string) {
	deps, err := s.store.ListReadyDeploymentsByEnvironment(envName)
	if err != nil {
		log.Printf("[rehydrate] error listing deployments for env %q: %v", envName, err)
		return
	}

	if len(deps) == 0 {
		log.Printf("[rehydrate] no ready deployments on environment %q", envName)
		return
	}

	log.Printf("[rehydrate] marking %d deployment(s) as impacted (env %q unhealthy)", len(deps), envName)

	for _, d := range deps {
		claimed, err := s.store.ClaimImpacted(d.ID)
		if err != nil {
			log.Printf("[rehydrate] deployment %s: error claiming: %v", d.ID, err)
			continue
		}
		if !claimed {
			log.Printf("[rehydrate] deployment %s: already impacted or rehydrating, skipping", d.ID)
			continue
		}

		s.store.AddHistory(&store.HistoryRecord{
			DeploymentID: d.ID,
			Action:       "impacted",
			Details:      mustJSON(map[string]any{
				"failedEnvironment": envName,
				"message":           fmt.Sprintf("environment %q became unhealthy", envName),
			}),
		})

		log.Printf("[rehydrate] deployment %s (app=%s): marked impacted", d.ID, d.ApplicationName)
	}
}

// handleDeploymentRehydrate handles POST /api/v1/deployments/{id}/rehydrate
// and triggers manual redeployment to a healthy environment.
func (s *Server) handleDeploymentRehydrate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.store.GetDeployment(id)
	if err != nil {
		handleStoreError(w, err, "deployment")
		return
	}

	if d.Status != "impacted" {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("can only rehydrate an impacted deployment, current status is %q", d.Status))
		return
	}

	go s.runRehydrate(d)

	d.Status = "rehydrating"
	s.store.UpdateDeployment(d)
	s.store.AddHistory(&store.HistoryRecord{DeploymentID: id, Action: "rehydrating"})

	writeJSON(w, http.StatusAccepted, d)
}

// runRehydrate destroys resources on the old environment and redeploys
// to a healthy one.
func (s *Server) runRehydrate(d *store.DeploymentRecord) {
	// Re-read to get the latest state.
	d, err := s.store.GetDeployment(d.ID)
	if err != nil {
		log.Printf("[rehydrate] deployment %s: error reading: %v", d.ID, err)
		return
	}

	log.Printf("[rehydrate] rehydrating deployment %s (app=%s)", d.ID, d.ApplicationName)

	// Phase 1: Destroy old resources.
	var state types.State
	if d.State != nil {
		json.Unmarshal(d.State, &state)
	}

	for name, resource := range state.Resources {
		lookupName := resource.Provider
		if resource.Environment != "" {
			lookupName = resource.Environment
		}
		provider, ok := s.registry.Get(lookupName)
		if !ok {
			log.Printf("[rehydrate] deployment %s: provider %q not found for resource %q, skipping destroy", d.ID, lookupName, name)
			continue
		}
		if err := provider.Destroy(resource); err != nil {
			log.Printf("[rehydrate] deployment %s: error destroying %q: %v (continuing)", d.ID, name, err)
		}
	}

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "destroyed_for_rehydration",
	})

	// Phase 2: Re-plan (state is cleared so scheduler picks a new env).
	appRec, err := s.store.GetApplication(d.ApplicationName)
	if err != nil {
		s.failDeployment(d, fmt.Sprintf("rehydration failed: application %q not found: %v", d.ApplicationName, err))
		return
	}

	app := recordToApplication(appRec)

	d.State = nil
	d.Status = "planning"
	s.store.UpdateDeployment(d)

	plan, err := s.computePlan(app, d)
	if err != nil {
		s.failDeployment(d, "rehydration planning failed: "+err.Error())
		return
	}

	planJSON, _ := json.Marshal(plan)
	d.Plan = planJSON
	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "rehydration_planned",
		Details:      planJSON,
	})

	// Phase 3: Apply.
	d.Status = "deploying"
	s.store.UpdateDeployment(d)

	newState := types.NewState(app.Metadata.Name)
	executor := engine.NewExecutor(s.registry)
	if err := executor.Execute(plan, newState); err != nil {
		stateJSON, _ := json.Marshal(newState)
		d.State = stateJSON
		s.failDeployment(d, "rehydration apply failed: "+err.Error())
		return
	}

	stateJSON, _ := json.Marshal(newState)
	d.State = stateJSON
	d.Status = "ready"
	s.store.UpdateDeployment(d)

	s.store.AddHistory(&store.HistoryRecord{
		DeploymentID: d.ID,
		Action:       "rehydrated",
		Details:      stateJSON,
	})

	log.Printf("[rehydrate] deployment %s: rehydration complete", d.ID)
}

// checkCompliance builds OPA inputs from the plan and evaluates them.
func (s *Server) checkCompliance(ctx context.Context, app *types.Application, plan *engine.Plan) ([]compliance.Violation, error) {
	componentMap := make(map[string]*types.Component)
	for i := range app.Spec.Components {
		c := &app.Spec.Components[i]
		componentMap[c.Name] = c
	}

	var inputs []compliance.StepInput
	for _, step := range plan.Steps {
		c := componentMap[step.Component]
		if c == nil {
			continue
		}

		envInput := compliance.EnvironmentInput{Name: step.Environment}
		// Try to enrich with environment metadata from the store.
		if envRec, err := s.store.GetEnvironment(step.Environment); err == nil {
			envInput.Provider = envRec.Provider
			envInput.Labels = envRec.Labels
			envInput.Capabilities = envRec.Capabilities
			if envRec.Cost != nil {
				var ci types.CostInfo
				if json.Unmarshal(envRec.Cost, &ci) == nil {
					envInput.Cost = &compliance.CostInput{
						Tier:       ci.Tier,
						HourlyRate: ci.HourlyRate,
					}
				}
			}
		}

		inputs = append(inputs, compliance.StepInput{
			Component: compliance.ComponentInput{
				Name:       c.Name,
				Type:       c.Type,
				Labels:     c.Labels,
				Properties: c.Properties,
				Requires:   c.Requires,
			},
			Environment: envInput,
			Action:      string(step.Diff.Action),
			Application: compliance.ApplicationInput{
				Name:   app.Metadata.Name,
				Labels: app.Metadata.Labels,
			},
		})
	}

	return s.compliance.EvaluateAll(ctx, inputs)
}

// handleComplianceCheck lets users validate a plan against compliance policies without deploying.
func (s *Server) handleComplianceCheck(w http.ResponseWriter, r *http.Request) {
	if s.compliance == nil || !s.compliance.HasPolicies() {
		var req struct {
			Application string `json:"application"`
		}
		decodeJSON(r, &req)
		writeJSON(w, http.StatusOK, map[string]any{
			"application": req.Application,
			"violations":  []any{},
			"passed":      true,
			"message":     "no compliance policies loaded",
		})
		return
	}

	var req struct {
		Application string `json:"application"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Application == "" {
		writeError(w, http.StatusBadRequest, "application is required")
		return
	}

	appRec, err := s.store.GetApplication(req.Application)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}

	app := recordToApplication(appRec)

	// Create a temporary deployment record for planning.
	tempDep := &store.DeploymentRecord{
		ID:              "compliance-check",
		ApplicationName: req.Application,
		Status:          "checking",
	}

	plan, err := s.computePlan(app, tempDep)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "planning failed: "+err.Error())
		return
	}

	violations, err := s.checkCompliance(r.Context(), app, plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compliance check error: "+err.Error())
		return
	}

	if violations == nil {
		violations = []compliance.Violation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"application": req.Application,
		"violations":  violations,
		"passed":      len(violations) == 0,
	})
}
