package scheduler

import (
	"fmt"
	"log"
	"slices"

	"github.com/dcm-io/dcm/pkg/provider"
	"github.com/dcm-io/dcm/pkg/types"
)

// EnvironmentInstance is a live environment with its provider.
type EnvironmentInstance struct {
	Env      types.Environment
	Provider types.Provider
}

// Registry manages environment instances.
type Registry struct {
	environments map[string]*EnvironmentInstance
	factories    *provider.FactoryRegistry
}

// NewRegistry creates a registry with the given provider factories.
func NewRegistry(factories *provider.FactoryRegistry) *Registry {
	return &Registry{
		environments: make(map[string]*EnvironmentInstance),
		factories:    factories,
	}
}

// RegisterEnvironment creates a provider from the environment config and registers it.
func (r *Registry) RegisterEnvironment(env types.Environment) error {
	if _, exists := r.environments[env.Metadata.Name]; exists {
		return fmt.Errorf("environment %q already registered", env.Metadata.Name)
	}

	p, err := r.factories.Create(env.Spec.Provider, env.Spec.Config)
	if err != nil {
		return fmt.Errorf("creating provider for environment %q: %w", env.Metadata.Name, err)
	}

	r.environments[env.Metadata.Name] = &EnvironmentInstance{
		Env:      env,
		Provider: p,
	}
	log.Printf("[registry] registered environment %q (provider=%s, capabilities=%v)",
		env.Metadata.Name, env.Spec.Provider, capStrings(p.Capabilities()))
	return nil
}

// RegisterProvider registers a bare provider as a default environment (backward compat).
// The environment name matches the provider name.
func (r *Registry) RegisterProvider(p types.Provider) {
	log.Printf("[registry] registered default environment %q (backward-compat, capabilities=%v)",
		p.Name(), capStrings(p.Capabilities()))
	r.environments[p.Name()] = &EnvironmentInstance{
		Env: types.Environment{
			Metadata: types.Metadata{Name: p.Name()},
			Spec: types.EnvironmentSpec{
				Provider: p.Name(),
			},
		},
		Provider: p,
	}
}

// GetEnvironment returns an environment instance by name.
func (r *Registry) GetEnvironment(name string) (*EnvironmentInstance, bool) {
	e, ok := r.environments[name]
	return e, ok
}

// Get returns a provider by environment name. Implements engine.ProviderRegistry.
func (r *Registry) Get(name string) (types.Provider, bool) {
	return r.GetProvider(name)
}

// GetProvider implements engine.ProviderRegistry for backward compatibility.
func (r *Registry) GetProvider(name string) (types.Provider, bool) {
	e, ok := r.environments[name]
	if !ok {
		return nil, false
	}
	return e.Provider, true
}

// GetForResource returns the first environment's provider that supports the resource type.
func (r *Registry) GetForResource(resourceType types.ResourceType) (types.Provider, error) {
	for _, e := range r.environments {
		if slices.Contains(e.Provider.Capabilities(), resourceType) {
			return e.Provider, nil
		}
	}
	return nil, fmt.Errorf("no environment found for resource type %q", resourceType)
}

// ListProviders returns all providers (one per environment).
func (r *Registry) ListProviders() []types.Provider {
	providers := make([]types.Provider, 0, len(r.environments))
	for _, e := range r.environments {
		providers = append(providers, e.Provider)
	}
	return providers
}

// ListEnvironments returns all registered environment instances.
func (r *Registry) ListEnvironments() []*EnvironmentInstance {
	envs := make([]*EnvironmentInstance, 0, len(r.environments))
	for _, e := range r.environments {
		envs = append(envs, e)
	}
	return envs
}

// ListByProvider returns environments backed by the given provider type.
func (r *Registry) ListByProvider(providerType string) []*EnvironmentInstance {
	var result []*EnvironmentInstance
	for _, e := range r.environments {
		if e.Env.Spec.Provider == providerType {
			result = append(result, e)
		}
	}
	return result
}

// ListByCapability returns environments whose provider supports the given resource type.
func (r *Registry) ListByCapability(rt types.ResourceType) []*EnvironmentInstance {
	var result []*EnvironmentInstance
	for _, e := range r.environments {
		if slices.Contains(e.Provider.Capabilities(), rt) {
			result = append(result, e)
		}
	}
	return result
}

// ListEnvironmentInfo returns summary info for all environments.
func (r *Registry) ListEnvironmentInfo() []EnvironmentInfo {
	var infos []EnvironmentInfo
	for _, e := range r.environments {
		infos = append(infos, EnvironmentInfo{
			Name:                    e.Env.Metadata.Name,
			Provider:                e.Env.Spec.Provider,
			Labels:                  e.Env.Metadata.Labels,
			Capabilities:            capStrings(e.Provider.Capabilities()),
			EnvironmentCapabilities: e.Env.Spec.Capabilities,
			Resources:               e.Env.Spec.Resources,
			Cost:                    e.Env.Spec.Cost,
		})
	}
	return infos
}

// EnvironmentInfo is a summary of an environment for API responses.
type EnvironmentInfo struct {
	Name                 string              `json:"name"`
	Provider             string              `json:"provider"`
	Labels               map[string]string   `json:"labels,omitempty"`
	Capabilities         []string            `json:"capabilities"`
	EnvironmentCapabilities []string         `json:"environmentCapabilities,omitempty"`
	Resources            *types.ResourcePool `json:"resources,omitempty"`
	Cost                 *types.CostInfo     `json:"cost,omitempty"`
}

func capStrings(caps []types.ResourceType) []string {
	out := make([]string, len(caps))
	for i, c := range caps {
		out[i] = string(c)
	}
	return out
}
