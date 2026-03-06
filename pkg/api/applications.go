package api

import (
	"encoding/json"
	"net/http"

	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

type createApplicationRequest struct {
	Name       string            `json:"name"`
	Labels     map[string]string `json:"labels"`
	Components []types.Component `json:"components"`
}

type validateApplicationResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func (s *Server) handleCreateApplication(w http.ResponseWriter, r *http.Request) {
	var req createApplicationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Components) == 0 {
		writeError(w, http.StatusBadRequest, "at least one component is required")
		return
	}

	// Validate the DAG.
	if _, err := engine.NewDAG(req.Components); err != nil {
		writeError(w, http.StatusBadRequest, "invalid component graph: "+err.Error())
		return
	}

	components, _ := json.Marshal(req.Components)
	rec := &store.ApplicationRecord{
		Name:       req.Name,
		Labels:     req.Labels,
		Components: components,
	}

	if err := s.store.CreateApplication(rec); err != nil {
		handleStoreError(w, err, "application")
		return
	}

	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleListApplications(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.ListApplications()
	if err != nil {
		handleStoreError(w, err, "applications")
		return
	}
	if apps == nil {
		apps = []store.ApplicationRecord{}
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) handleGetApplication(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	app, err := s.store.GetApplication(name)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleUpdateApplication(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req createApplicationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if len(req.Components) == 0 {
		writeError(w, http.StatusBadRequest, "at least one component is required")
		return
	}

	if _, err := engine.NewDAG(req.Components); err != nil {
		writeError(w, http.StatusBadRequest, "invalid component graph: "+err.Error())
		return
	}

	components, _ := json.Marshal(req.Components)
	rec := &store.ApplicationRecord{
		Name:       name,
		Labels:     req.Labels,
		Components: components,
	}

	if err := s.store.UpdateApplication(rec); err != nil {
		handleStoreError(w, err, "application")
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleDeleteApplication(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.store.DeleteApplication(name); err != nil {
		handleStoreError(w, err, "application")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleValidateApplication(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	app, err := s.store.GetApplication(name)
	if err != nil {
		handleStoreError(w, err, "application")
		return
	}

	var components []types.Component
	json.Unmarshal(app.Components, &components)

	var errs []string
	if _, err := engine.NewDAG(components); err != nil {
		errs = append(errs, "DAG: "+err.Error())
	}

	resp := validateApplicationResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
	writeJSON(w, http.StatusOK, resp)
}
