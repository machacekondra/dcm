package api

import (
	"encoding/json"
	"net/http"

	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

type createPolicyRequest struct {
	Name  string             `json:"name"`
	Rules []types.PolicyRule `json:"rules"`
}

type validatePolicyResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

type evaluatePoliciesRequest struct {
	Application string   `json:"application"`
	Policies    []string `json:"policies"`
}

type evaluateComponentResult struct {
	Component    string         `json:"component"`
	Type         string         `json:"type"`
	MatchedRules []string       `json:"matchedRules"`
	Required     string         `json:"required,omitempty"`
	Preferred    []string       `json:"preferred,omitempty"`
	Forbidden    []string       `json:"forbidden,omitempty"`
	Strategy     string         `json:"strategy,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
	Selected     string         `json:"selected,omitempty"`
	Error        string         `json:"error,omitempty"`
}

func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req createPolicyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Validate by attempting to build an evaluator.
	if err := validateRules(req.Rules); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rules, _ := json.Marshal(req.Rules)
	rec := &store.PolicyRecord{
		Name:  req.Name,
		Rules: rules,
	}

	if err := s.store.CreatePolicy(rec); err != nil {
		handleStoreError(w, err, "policy")
		return
	}

	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.store.ListPolicies()
	if err != nil {
		handleStoreError(w, err, "policies")
		return
	}
	if policies == nil {
		policies = []store.PolicyRecord{}
	}
	writeJSON(w, http.StatusOK, policies)
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.store.GetPolicy(name)
	if err != nil {
		handleStoreError(w, err, "policy")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req createPolicyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := validateRules(req.Rules); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rules, _ := json.Marshal(req.Rules)
	rec := &store.PolicyRecord{
		Name:  name,
		Rules: rules,
	}

	if err := s.store.UpdatePolicy(rec); err != nil {
		handleStoreError(w, err, "policy")
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.store.DeletePolicy(name); err != nil {
		handleStoreError(w, err, "policy")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleValidatePolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.store.GetPolicy(name)
	if err != nil {
		handleStoreError(w, err, "policy")
		return
	}

	var rules []types.PolicyRule
	json.Unmarshal(p.Rules, &rules)

	var errs []string
	if err := validateRules(rules); err != nil {
		errs = append(errs, err.Error())
	}

	resp := validatePolicyResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEvaluatePolicies(w http.ResponseWriter, r *http.Request) {
	var req evaluatePoliciesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Application == "" {
		writeError(w, http.StatusBadRequest, "application is required")
		return
	}

	// Load the application.
	appRec, err := s.store.GetApplication(req.Application)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}

	app := recordToApplication(appRec)

	// Load requested policies.
	policyTypes, err := s.loadPolicies(req.Policies)
	if err != nil {
		handleStoreError(w, err, "policy")
		return
	}

	eval, err := policy.NewEvaluator(policyTypes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "policy evaluation error: "+err.Error())
		return
	}

	var results []evaluateComponentResult
	for _, comp := range app.Spec.Components {
		result, err := eval.Evaluate(&comp, app)
		ecr := evaluateComponentResult{
			Component: comp.Name,
			Type:      comp.Type,
		}

		if err != nil {
			ecr.Error = err.Error()
		} else {
			ecr.MatchedRules = result.MatchedRules
			ecr.Required = result.Required
			ecr.Preferred = result.Preferred
			ecr.Forbidden = result.Forbidden
			ecr.Strategy = result.Strategy
			ecr.Properties = result.Properties

			selected, err := policy.SelectProvider(result, s.registry, types.ResourceType(comp.Type))
			if err != nil {
				ecr.Error = err.Error()
			} else {
				ecr.Selected = selected.Name()
			}
		}

		results = append(results, ecr)
	}

	writeJSON(w, http.StatusOK, results)
}

func validateRules(rules []types.PolicyRule) error {
	testPolicy := types.Policy{
		Metadata: types.Metadata{Name: "validation"},
		Spec:     types.PolicySpec{Rules: rules},
	}
	_, err := policy.NewEvaluator([]types.Policy{testPolicy})
	return err
}
