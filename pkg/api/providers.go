package api

import (
	"net/http"

	"github.com/dcm-io/dcm/pkg/types"
)

type providerInfo struct {
	Name         string             `json:"name"`
	Capabilities []types.ResourceType `json:"capabilities"`
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := s.registry.ListProviders()
	var infos []providerInfo
	for _, p := range providers {
		infos = append(infos, providerInfo{
			Name:         p.Name(),
			Capabilities: p.Capabilities(),
		})
	}
	if infos == nil {
		infos = []providerInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}
