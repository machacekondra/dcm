package engine

import (
	"fmt"
	"log"

	"github.com/dcm-io/dcm/pkg/types"
)

// Executor applies a plan by calling the appropriate providers.
type Executor struct {
	registry ProviderRegistry
}

// NewExecutor creates a new executor with the given provider registry.
func NewExecutor(registry ProviderRegistry) *Executor {
	return &Executor{registry: registry}
}

// Execute runs all steps in the plan sequentially.
// It updates the state as each step completes.
func (e *Executor) Execute(plan *Plan, state *types.State) error {
	log.Printf("[executor] executing plan for app=%q, %d step(s)", plan.AppName, len(plan.Steps))

	for _, step := range plan.Steps {
		if step.Diff.Action == types.DiffActionNone {
			log.Printf("[executor]   skip %q (no changes)", step.Component)
			continue
		}

		// Look up provider by environment name first, then fall back to provider type name.
		lookupName := step.Diff.Provider
		if step.Environment != "" {
			lookupName = step.Environment
		}
		provider, ok := e.registry.Get(lookupName)
		if !ok {
			return fmt.Errorf("provider %q not found for component %q", lookupName, step.Component)
		}

		log.Printf("[executor]   %s %q (type=%s provider=%s env=%s)",
			step.Diff.Action, step.Component, step.Diff.Type, step.Diff.Provider, step.Environment)

		switch step.Diff.Action {
		case types.DiffActionCreate, types.DiffActionUpdate:
			resource, err := provider.Apply(step.Diff)
			if err != nil {
				log.Printf("[executor]   FAILED %s %q: %v", step.Diff.Action, step.Component, err)
				return fmt.Errorf("applying %s: %w", step.Component, err)
			}
			// Ensure environment is persisted so destroy can look up the right provider.
			if resource.Environment == "" && step.Environment != "" {
				resource.Environment = step.Environment
			}
			log.Printf("[executor]   completed %s %q → status=%s", step.Diff.Action, step.Component, resource.Status)
			state.Resources[step.Component] = resource

		case types.DiffActionDelete:
			existing := state.Resources[step.Component]
			if existing != nil {
				if err := provider.Destroy(existing); err != nil {
					log.Printf("[executor]   FAILED destroy %q: %v", step.Component, err)
					return fmt.Errorf("destroying %s: %w", step.Component, err)
				}
				log.Printf("[executor]   destroyed %q", step.Component)
				delete(state.Resources, step.Component)
			}
		}
	}

	log.Printf("[executor] execution complete")
	return nil
}
