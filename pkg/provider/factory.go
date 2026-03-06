package provider

import (
	"fmt"
	"log"

	"github.com/dcm-io/dcm/pkg/types"
)

// ProviderFactory creates a provider instance from config.
type ProviderFactory func(config map[string]any) (types.Provider, error)

// FactoryRegistry manages provider factories by provider type name.
type FactoryRegistry struct {
	factories map[string]ProviderFactory
}

// NewFactoryRegistry creates a factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{factories: make(map[string]ProviderFactory)}
}

// Register adds a factory for a provider type.
func (r *FactoryRegistry) Register(providerType string, factory ProviderFactory) {
	log.Printf("[factory] registered provider type %q", providerType)
	r.factories[providerType] = factory
}

// Create creates a provider instance from the given type and config.
func (r *FactoryRegistry) Create(providerType string, config map[string]any) (types.Provider, error) {
	factory, ok := r.factories[providerType]
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
	log.Printf("[factory] creating provider instance type=%q config=%v", providerType, config)
	p, err := factory(config)
	if err != nil {
		log.Printf("[factory] failed to create provider type=%q: %v", providerType, err)
		return nil, err
	}
	log.Printf("[factory] created provider %q (type=%s)", p.Name(), providerType)
	return p, nil
}

// Has checks if a factory exists for the given provider type.
func (r *FactoryRegistry) Has(providerType string) bool {
	_, ok := r.factories[providerType]
	return ok
}

// Types returns all registered provider type names.
func (r *FactoryRegistry) Types() []string {
	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}
