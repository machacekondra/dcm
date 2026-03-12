package api

import (
	"encoding/json"
	"net/http"

	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

type createEnvironmentRequest struct {
	Name         string              `json:"name"`
	Provider     string              `json:"provider"`
	Labels       map[string]string   `json:"labels,omitempty"`
	Capabilities []string            `json:"capabilities,omitempty"`
	Config       map[string]any      `json:"config,omitempty"`
	Resources    *types.ResourcePool `json:"resources,omitempty"`
	Cost         *types.CostInfo     `json:"cost,omitempty"`
	HealthCheck  *types.HealthCheck  `json:"healthCheck,omitempty"`
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

	var resources, cost, healthCheck json.RawMessage
	if req.Resources != nil {
		r, _ := json.Marshal(req.Resources)
		resources = r
	}
	if req.Cost != nil {
		c, _ := json.Marshal(req.Cost)
		cost = c
	}
	if req.HealthCheck != nil {
		h, _ := json.Marshal(req.HealthCheck)
		healthCheck = h
	}

	rec := &store.EnvironmentRecord{
		Name:         req.Name,
		Provider:     req.Provider,
		Labels:       req.Labels,
		Capabilities: req.Capabilities,
		Config:       req.Config,
		Resources:    resources,
		Cost:         cost,
		HealthCheck:  healthCheck,
		Status:       "active",
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

	var resources, cost, healthCheck json.RawMessage
	if req.Resources != nil {
		r, _ := json.Marshal(req.Resources)
		resources = r
	}
	if req.Cost != nil {
		c, _ := json.Marshal(req.Cost)
		cost = c
	}
	if req.HealthCheck != nil {
		h, _ := json.Marshal(req.HealthCheck)
		healthCheck = h
	}

	rec := &store.EnvironmentRecord{
		Name:         name,
		Provider:     req.Provider,
		Labels:       req.Labels,
		Capabilities: req.Capabilities,
		Config:       req.Config,
		Resources:    resources,
		Cost:         cost,
		HealthCheck:  healthCheck,
		Status:       "active",
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

func (s *Server) handleEnvironmentHeartbeat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req struct {
		Status  string `json:"status"`
		Message string `json:"message,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Status == "" {
		req.Status = "healthy"
	}

	if req.Status != "healthy" && req.Status != "unhealthy" && req.Status != "degraded" {
		writeError(w, http.StatusBadRequest, "status must be healthy, unhealthy, or degraded")
		return
	}

	if err := s.store.UpdateHealthStatus(name, req.Status, req.Message); err != nil {
		handleStoreError(w, err, "environment")
		return
	}

	// Update in-memory registry so scheduler sees the change immediately.
	s.registry.UpdateHealthStatus(name, req.Status)

	env, err := s.store.GetEnvironment(name)
	if err != nil {
		handleStoreError(w, err, "environment")
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handleEnvironmentHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	env, err := s.store.GetEnvironment(name)
	if err != nil {
		handleStoreError(w, err, "environment")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":          env.Name,
		"healthStatus":  env.HealthStatus,
		"healthMessage": env.HealthMessage,
		"lastHeartbeat": env.LastHeartbeat,
	})
}
