package engine

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/types"
)

// Plan represents the full execution plan for an application.
type Plan struct {
	AppName string        `json:"appName"`
	Steps   []PlanStep    `json:"steps"`
}

// PlanStep is a single step in the plan, corresponding to one component.
type PlanStep struct {
	Component string      `json:"component"`
	Diff      *types.Diff `json:"diff"`
}

// Planner computes the execution plan for an application.
type Planner struct {
	registry ProviderRegistry
}

// ProviderRegistry looks up providers by name.
type ProviderRegistry interface {
	Get(name string) (types.Provider, bool)
	GetForResource(resourceType types.ResourceType) (types.Provider, error)
}

// NewPlanner creates a new planner with the given provider registry.
func NewPlanner(registry ProviderRegistry) *Planner {
	return &Planner{registry: registry}
}

// CreatePlan builds a plan by comparing desired state (from the app spec)
// against current state.
func (p *Planner) CreatePlan(app *types.Application, currentState *types.State) (*Plan, error) {
	dag, err := NewDAG(app.Spec.Components)
	if err != nil {
		return nil, fmt.Errorf("building DAG: %w", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("sorting DAG: %w", err)
	}

	plan := &Plan{
		AppName: app.Metadata.Name,
		Steps:   make([]PlanStep, 0, len(sorted)),
	}

	for _, component := range sorted {
		provider, err := p.registry.GetForResource(types.ResourceType(component.Type))
		if err != nil {
			return nil, fmt.Errorf("finding provider for %s (type %s): %w", component.Name, component.Type, err)
		}

		desired := &types.Resource{
			Name:       component.Name,
			Type:       types.ResourceType(component.Type),
			Provider:   provider.Name(),
			Properties: component.Properties,
			Status:     types.ResourceStatusPending,
		}

		var current *types.Resource
		if currentState != nil {
			current = currentState.Resources[component.Name]
		}

		diff, err := provider.Plan(desired, current)
		if err != nil {
			return nil, fmt.Errorf("planning %s: %w", component.Name, err)
		}

		plan.Steps = append(plan.Steps, PlanStep{
			Component: component.Name,
			Diff:      diff,
		})
	}

	return plan, nil
}
