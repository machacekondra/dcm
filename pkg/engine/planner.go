package engine

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/types"
)

// Plan represents the full execution plan for an application.
type Plan struct {
	AppName string     `json:"appName"`
	Steps   []PlanStep `json:"steps"`
}

// PlanStep is a single step in the plan, corresponding to one component.
type PlanStep struct {
	Component    string         `json:"component"`
	Diff         *types.Diff    `json:"diff"`
	MatchedRules []string       `json:"matchedRules,omitempty"`
}

// ProviderRegistry looks up providers by name or resource type.
type ProviderRegistry interface {
	Get(name string) (types.Provider, bool)
	GetForResource(resourceType types.ResourceType) (types.Provider, error)
	ListProviders() []types.Provider
}

// Planner computes the execution plan for an application.
type Planner struct {
	registry  ProviderRegistry
	evaluator *policy.Evaluator
}

// NewPlanner creates a new planner with the given provider registry.
// The policy evaluator is optional — pass nil to skip policy evaluation.
func NewPlanner(registry ProviderRegistry, evaluator *policy.Evaluator) *Planner {
	return &Planner{registry: registry, evaluator: evaluator}
}

// CreatePlan builds a plan by comparing desired state (from the app spec)
// against current state. If a policy evaluator is set, it is used to select
// providers and merge properties.
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
		resourceType := types.ResourceType(component.Type)
		properties := copyProperties(component.Properties)

		var provider types.Provider
		var matchedRules []string

		if p.evaluator != nil {
			result, err := p.evaluator.Evaluate(component, app)
			if err != nil {
				return nil, fmt.Errorf("evaluating policies for %s: %w", component.Name, err)
			}

			matchedRules = result.MatchedRules

			// Merge policy-injected properties (component properties take precedence).
			for k, v := range result.Properties {
				if _, exists := properties[k]; !exists {
					properties[k] = v
				}
			}

			// Select provider using policy result.
			provider, err = policy.SelectProvider(result, p.registry, resourceType)
			if err != nil {
				return nil, fmt.Errorf("selecting provider for %s (type %s): %w", component.Name, component.Type, err)
			}
		} else {
			// No policies — fall back to first capable provider.
			provider, err = p.registry.GetForResource(resourceType)
			if err != nil {
				return nil, fmt.Errorf("finding provider for %s (type %s): %w", component.Name, component.Type, err)
			}
		}

		desired := &types.Resource{
			Name:       component.Name,
			Type:       resourceType,
			Provider:   provider.Name(),
			Properties: properties,
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
			Component:    component.Name,
			Diff:         diff,
			MatchedRules: matchedRules,
		})
	}

	return plan, nil
}

func copyProperties(props map[string]any) map[string]any {
	if props == nil {
		return make(map[string]any)
	}
	out := make(map[string]any, len(props))
	for k, v := range props {
		out[k] = v
	}
	return out
}
