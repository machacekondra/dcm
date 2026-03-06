package scheduler

import (
	"fmt"
	"math"
	"slices"

	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/types"
	"github.com/google/cel-go/cel"
)

// Scheduler selects the best environment for a component based on policies.
type Scheduler struct {
	registry  *Registry
	evaluator *policy.Evaluator
	celEnv    *cel.Env
	// roundRobinIndex tracks round-robin state per scheduling call batch.
	roundRobinIndex int
}

// NewScheduler creates a scheduler with the given registry and optional policy evaluator.
func NewScheduler(registry *Registry, evaluator *policy.Evaluator) (*Scheduler, error) {
	env, err := cel.NewEnv(
		cel.Variable("environment", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating environment CEL env: %w", err)
	}

	return &Scheduler{
		registry:  registry,
		evaluator: evaluator,
		celEnv:    env,
	}, nil
}

// ScheduleResult is the outcome of scheduling a single component.
type ScheduleResult struct {
	Environment  string   // selected environment name
	ProviderType string   // provider type name
	Provider     types.Provider
	MatchedRules []string
	Strategy     string
	Properties   map[string]any
}

// Schedule selects the best environment for a component.
func (s *Scheduler) Schedule(component *types.Component, app *types.Application) (*ScheduleResult, error) {
	resourceType := types.ResourceType(component.Type)

	// Start with all environments that support the component's resource type.
	candidates := s.registry.ListByCapability(resourceType)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no environment supports resource type %q", component.Type)
	}

	var matchedRules []string
	var properties map[string]any
	strategy := "first"

	// If we have a policy evaluator, use it to filter and constrain.
	if s.evaluator != nil {
		result, err := s.evaluator.Evaluate(component, app)
		if err != nil {
			return nil, fmt.Errorf("evaluating policies for %s: %w", component.Name, err)
		}

		matchedRules = result.MatchedRules
		properties = result.Properties

		// Determine strategy: environment strategy overrides provider strategy.
		if result.Environments.Strategy != "" {
			strategy = result.Environments.Strategy
		} else if result.Strategy != "" {
			strategy = result.Strategy
		}

		// Filter by provider-level constraints.
		candidates, err = s.filterByProviderPolicy(candidates, result)
		if err != nil {
			return nil, err
		}

		// Filter by environment-level constraints.
		candidates, err = s.filterByEnvironmentPolicy(candidates, result)
		if err != nil {
			return nil, err
		}

		// Filter by environment match expression (CEL).
		if result.Environments.MatchExpression != "" {
			candidates, err = s.filterByMatchExpression(candidates, result.Environments.MatchExpression)
			if err != nil {
				return nil, fmt.Errorf("evaluating environment matchExpression: %w", err)
			}
		}

		// Order by environment preferred list, then provider preferred list.
		candidates = s.orderByPreferred(candidates, result)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no environment available for component %q (type %s) after applying policies",
			component.Name, component.Type)
	}

	// Apply scheduling strategy.
	selected, err := s.applyStrategy(candidates, strategy)
	if err != nil {
		return nil, err
	}

	return &ScheduleResult{
		Environment:  selected.Env.Metadata.Name,
		ProviderType: selected.Env.Spec.Provider,
		Provider:     selected.Provider,
		MatchedRules: matchedRules,
		Strategy:     strategy,
		Properties:   properties,
	}, nil
}

// filterByProviderPolicy removes environments whose provider type is forbidden,
// or keeps only required provider type.
func (s *Scheduler) filterByProviderPolicy(candidates []*EnvironmentInstance, result *policy.Result) ([]*EnvironmentInstance, error) {
	// Required provider type.
	if result.Required != "" {
		var filtered []*EnvironmentInstance
		for _, c := range candidates {
			if c.Env.Spec.Provider == result.Required {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("required provider type %q has no matching environments", result.Required)
		}
		return filtered, nil
	}

	// Forbidden provider types.
	if len(result.Forbidden) > 0 {
		var filtered []*EnvironmentInstance
		for _, c := range candidates {
			if !slices.Contains(result.Forbidden, c.Env.Spec.Provider) {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
	}

	return candidates, nil
}

// filterByEnvironmentPolicy removes environments that are forbidden by name
// or keeps only the required environment.
func (s *Scheduler) filterByEnvironmentPolicy(candidates []*EnvironmentInstance, result *policy.Result) ([]*EnvironmentInstance, error) {
	envResult := result.Environments

	// Required environment.
	if envResult.Required != "" {
		for _, c := range candidates {
			if c.Env.Metadata.Name == envResult.Required {
				return []*EnvironmentInstance{c}, nil
			}
		}
		return nil, fmt.Errorf("required environment %q not found or filtered out", envResult.Required)
	}

	// Forbidden environments.
	if len(envResult.Forbidden) > 0 {
		var filtered []*EnvironmentInstance
		for _, c := range candidates {
			if !slices.Contains(envResult.Forbidden, c.Env.Metadata.Name) {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
	}

	return candidates, nil
}

// filterByMatchExpression evaluates a CEL expression against each environment.
func (s *Scheduler) filterByMatchExpression(candidates []*EnvironmentInstance, expr string) ([]*EnvironmentInstance, error) {
	ast, issues := s.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	prog, err := s.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("creating CEL program: %w", err)
	}

	var filtered []*EnvironmentInstance
	for _, c := range candidates {
		envMap := buildEnvironmentCELMap(c)
		out, _, err := prog.Eval(map[string]any{"environment": envMap})
		if err != nil {
			continue // skip environments that cause evaluation errors
		}
		if v, ok := out.Value().(bool); ok && v {
			filtered = append(filtered, c)
		}
	}

	return filtered, nil
}

// orderByPreferred reorders candidates: environment preferred first, then provider preferred.
func (s *Scheduler) orderByPreferred(candidates []*EnvironmentInstance, result *policy.Result) []*EnvironmentInstance {
	envPreferred := result.Environments.Preferred
	provPreferred := result.Preferred

	if len(envPreferred) == 0 && len(provPreferred) == 0 {
		return candidates
	}

	// Score: lower is better.
	score := func(c *EnvironmentInstance) int {
		// Check environment preferred list first.
		for i, name := range envPreferred {
			if c.Env.Metadata.Name == name {
				return i
			}
		}
		// Then provider preferred list.
		for i, name := range provPreferred {
			if c.Env.Spec.Provider == name {
				return len(envPreferred) + i
			}
		}
		return len(envPreferred) + len(provPreferred)
	}

	sorted := make([]*EnvironmentInstance, len(candidates))
	copy(sorted, candidates)
	slices.SortStableFunc(sorted, func(a, b *EnvironmentInstance) int {
		return score(a) - score(b)
	})

	return sorted
}

// applyStrategy selects from candidates using the given strategy.
func (s *Scheduler) applyStrategy(candidates []*EnvironmentInstance, strategy string) (*EnvironmentInstance, error) {
	switch strategy {
	case "first", "":
		return candidates[0], nil

	case "cheapest":
		return s.selectCheapest(candidates), nil

	case "least-loaded":
		return s.selectLeastLoaded(candidates), nil

	case "round-robin":
		selected := candidates[s.roundRobinIndex%len(candidates)]
		s.roundRobinIndex++
		return selected, nil

	case "bin-pack":
		return s.selectBinPack(candidates), nil

	default:
		return nil, fmt.Errorf("unknown scheduling strategy: %s", strategy)
	}
}

func (s *Scheduler) selectCheapest(candidates []*EnvironmentInstance) *EnvironmentInstance {
	best := candidates[0]
	bestRate := math.MaxFloat64

	for _, c := range candidates {
		rate := math.MaxFloat64
		if c.Env.Spec.Cost != nil {
			rate = c.Env.Spec.Cost.HourlyRate
		}
		if rate < bestRate {
			bestRate = rate
			best = c
		}
	}
	return best
}

func (s *Scheduler) selectLeastLoaded(candidates []*EnvironmentInstance) *EnvironmentInstance {
	best := candidates[0]
	bestCapacity := -1

	for _, c := range candidates {
		capacity := 0
		if c.Env.Spec.Resources != nil {
			capacity = c.Env.Spec.Resources.CPU + c.Env.Spec.Resources.Memory + c.Env.Spec.Resources.Pods
		}
		if capacity > bestCapacity {
			bestCapacity = capacity
			best = c
		}
	}
	return best
}

func (s *Scheduler) selectBinPack(candidates []*EnvironmentInstance) *EnvironmentInstance {
	// Bin-pack: prefer the environment with the LEAST available capacity
	// (fill up existing before using new).
	best := candidates[0]
	bestCapacity := math.MaxInt

	for _, c := range candidates {
		capacity := math.MaxInt
		if c.Env.Spec.Resources != nil {
			capacity = c.Env.Spec.Resources.CPU + c.Env.Spec.Resources.Memory + c.Env.Spec.Resources.Pods
		}
		if capacity < bestCapacity {
			bestCapacity = capacity
			best = c
		}
	}
	return best
}

func buildEnvironmentCELMap(e *EnvironmentInstance) map[string]any {
	labels := map[string]any{}
	for k, v := range e.Env.Metadata.Labels {
		labels[k] = v
	}

	resources := map[string]any{
		"cpu":    0,
		"memory": 0,
		"pods":   0,
	}
	if e.Env.Spec.Resources != nil {
		resources["cpu"] = e.Env.Spec.Resources.CPU
		resources["memory"] = e.Env.Spec.Resources.Memory
		resources["pods"] = e.Env.Spec.Resources.Pods
	}

	cost := map[string]any{
		"tier":       "",
		"hourlyRate": 0.0,
	}
	if e.Env.Spec.Cost != nil {
		cost["tier"] = e.Env.Spec.Cost.Tier
		cost["hourlyRate"] = e.Env.Spec.Cost.HourlyRate
	}

	return map[string]any{
		"name":      e.Env.Metadata.Name,
		"provider":  e.Env.Spec.Provider,
		"labels":    labels,
		"resources": resources,
		"cost":      cost,
	}
}
