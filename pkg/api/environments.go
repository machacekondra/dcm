package api

import (
	"encoding/json"
	"net/http"

	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

type createEnvironmentRequest struct {
	Name      string              `json:"name"`
	Provider  string              `json:"provider"`
	Labels    map[string]string   `json:"labels,omitempty"`
	Config    map[string]any      `json:"config,omitempty"`
	Resources *types.ResourcePool `json:"resources,omitempty"`
	Cost      *types.CostInfo     `json:"cost,omitempty"`
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	var req createEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	var resources, cost json.RawMessage
	if req.Resources != nil {
		r, _ := json.Marshal(req.Resources)
		resources = r
	}
	if req.Cost != nil {
		c, _ := json.Marshal(req.Cost)
		cost = c
	}

	rec := &store.EnvironmentRecord{
		Name:      req.Name,
		Provider:  req.Provider,
		Labels:    req.Labels,
		Config:    req.Config,
		Resources: resources,
		Cost:      cost,
		Status:    "active",
	}

	if err := s.store.CreateEnvironment(rec); err != nil {
		handleStoreError(w, err, "environment")
		return
	}

	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	envs, err := s.store.ListEnvironments()
	if err != nil {
		handleStoreError(w, err, "environments")
		return
	}
	if envs == nil {
		envs = []store.EnvironmentRecord{}
	}
	writeJSON(w, http.StatusOK, envs)
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	env, err := s.store.GetEnvironment(name)
	if err != nil {
		handleStoreError(w, err, "environment")
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req createEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	var resources, cost json.RawMessage
	if req.Resources != nil {
		r, _ := json.Marshal(req.Resources)
		resources = r
	}
	if req.Cost != nil {
		c, _ := json.Marshal(req.Cost)
		cost = c
	}

	rec := &store.EnvironmentRecord{
		Name:      name,
		Provider:  req.Provider,
		Labels:    req.Labels,
		Config:    req.Config,
		Resources: resources,
		Cost:      cost,
		Status:    "active",
	}

	if err := s.store.UpdateEnvironment(rec); err != nil {
		handleStoreError(w, err, "environment")
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.store.DeleteEnvironment(name); err != nil {
		handleStoreError(w, err, "environment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
