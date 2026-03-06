package engine

import (
	"fmt"

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
	for _, step := range plan.Steps {
		if step.Diff.Action == types.DiffActionNone {
			continue
		}

		provider, ok := e.registry.Get(step.Diff.Provider)
		if !ok {
			return fmt.Errorf("provider %q not found for component %q", step.Diff.Provider, step.Component)
		}

		fmt.Printf("  %s %s (%s via %s)\n", step.Diff.Action, step.Component, step.Diff.Type, step.Diff.Provider)

		switch step.Diff.Action {
		case types.DiffActionCreate, types.DiffActionUpdate:
			resource, err := provider.Apply(step.Diff)
			if err != nil {
				return fmt.Errorf("applying %s: %w", step.Component, err)
			}
			state.Resources[step.Component] = resource

		case types.DiffActionDelete:
			existing := state.Resources[step.Component]
			if existing != nil {
				if err := provider.Destroy(existing); err != nil {
					return fmt.Errorf("destroying %s: %w", step.Component, err)
				}
				delete(state.Resources, step.Component)
			}
		}
	}
	return nil
}
