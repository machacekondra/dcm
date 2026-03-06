package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

// Server is the DCM API server.
type Server struct {
	store    *store.Store
	registry engine.ProviderRegistry
	mux      *http.ServeMux
}

// NewServer creates a new API server.
func NewServer(st *store.Store, registry engine.ProviderRegistry) *Server {
	s := &Server{
		store:    st,
		registry: registry,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Applications
	s.mux.HandleFunc("POST /api/v1/applications", s.handleCreateApplication)
	s.mux.HandleFunc("GET /api/v1/applications", s.handleListApplications)
	s.mux.HandleFunc("GET /api/v1/applications/{name}", s.handleGetApplication)
	s.mux.HandleFunc("PUT /api/v1/applications/{name}", s.handleUpdateApplication)
	s.mux.HandleFunc("DELETE /api/v1/applications/{name}", s.handleDeleteApplication)
	s.mux.HandleFunc("POST /api/v1/applications/{name}/validate", s.handleValidateApplication)

	// Policies
	s.mux.HandleFunc("POST /api/v1/policies", s.handleCreatePolicy)
	s.mux.HandleFunc("GET /api/v1/policies", s.handleListPolicies)
	s.mux.HandleFunc("GET /api/v1/policies/{name}", s.handleGetPolicy)
	s.mux.HandleFunc("PUT /api/v1/policies/{name}", s.handleUpdatePolicy)
	s.mux.HandleFunc("DELETE /api/v1/policies/{name}", s.handleDeletePolicy)
	s.mux.HandleFunc("POST /api/v1/policies/{name}/validate", s.handleValidatePolicy)
	s.mux.HandleFunc("POST /api/v1/policies/evaluate", s.handleEvaluatePolicies)

	// Deployments
	s.mux.HandleFunc("POST /api/v1/deployments", s.handleCreateDeployment)
	s.mux.HandleFunc("GET /api/v1/deployments", s.handleListDeployments)
	s.mux.HandleFunc("GET /api/v1/deployments/{id}", s.handleGetDeployment)
	s.mux.HandleFunc("DELETE /api/v1/deployments/{id}", s.handleDeleteDeployment)
	s.mux.HandleFunc("POST /api/v1/deployments/{id}/plan", s.handleDeploymentPlan)
	s.mux.HandleFunc("GET /api/v1/deployments/{id}/history", s.handleDeploymentHistory)

	// Providers
	s.mux.HandleFunc("GET /api/v1/providers", s.handleListProviders)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers for UI consumption.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	log.Printf("%s %s", r.Method, r.URL.Path)
	s.mux.ServeHTTP(w, r)
}

// Start runs the server on the given address.
func (s *Server) Start(addr string) error {
	log.Printf("DCM API server listening on %s", addr)
	return http.ListenAndServe(addr, s)
}

// --- Helpers ---

// recordToApplication converts a store record back to the engine's Application type.
func recordToApplication(rec *store.ApplicationRecord) *types.Application {
	var components []types.Component
	json.Unmarshal(rec.Components, &components)

	return &types.Application{
		APIVersion: "dcm.io/v1",
		Kind:       "Application",
		Metadata: types.Metadata{
			Name:   rec.Name,
			Labels: rec.Labels,
		},
		Spec: types.ApplicationSpec{
			Components: components,
		},
	}
}

// loadPolicies loads policy records by name and converts them to types.Policy.
func (s *Server) loadPolicies(names []string) ([]types.Policy, error) {
	var policies []types.Policy
	for _, name := range names {
		rec, err := s.store.GetPolicy(name)
		if err != nil {
			return nil, fmt.Errorf("policy %q: %w", name, err)
		}

		var rules []types.PolicyRule
		json.Unmarshal(rec.Rules, &rules)

		policies = append(policies, types.Policy{
			APIVersion: "dcm.io/v1",
			Kind:       "Policy",
			Metadata:   types.Metadata{Name: rec.Name},
			Spec:       types.PolicySpec{Rules: rules},
		})
	}
	return policies, nil
}
