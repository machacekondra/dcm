package provider

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/types"
)

// Registry manages available providers and maps resource types to them.
type Registry struct {
	providers map[string]types.Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]types.Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p types.Provider) {
	r.providers[p.Name()] = p
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (types.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// GetForResource finds the first provider that supports the given resource type.
func (r *Registry) GetForResource(resourceType types.ResourceType) (types.Provider, error) {
	for _, p := range r.providers {
		for _, cap := range p.Capabilities() {
			if cap == resourceType {
				return p, nil
			}
		}
	}
	return nil, fmt.Errorf("no provider found for resource type %q", resourceType)
}

// List returns all registered provider names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// ListProviders returns all registered providers.
func (r *Registry) ListProviders() []types.Provider {
	providers := make([]types.Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}
