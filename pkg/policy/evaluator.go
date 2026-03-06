package policy

import (
	"fmt"
	"slices"
	"sort"

	"github.com/dcm-io/dcm/pkg/types"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

// Evaluator evaluates policies against components to determine provider selection
// and property overrides.
type Evaluator struct {
	policies []types.Policy
	celEnv   *cel.Env
}

// NewEvaluator creates a policy evaluator with the given policies.
// It pre-compiles the CEL environment used for expression-based matching.
func NewEvaluator(policies []types.Policy) (*Evaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable("component", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("app", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating CEL environment: %w", err)
	}

	// Validate all CEL expressions at construction time.
	for _, policy := range policies {
		for i, rule := range policy.Spec.Rules {
			if rule.Match.Expression != "" {
				_, issues := env.Compile(rule.Match.Expression)
				if issues != nil && issues.Err() != nil {
					ruleName := rule.Name
					if ruleName == "" {
						ruleName = fmt.Sprintf("rule[%d]", i)
					}
					return nil, fmt.Errorf("policy %q, %s: invalid CEL expression: %w",
						policy.Metadata.Name, ruleName, issues.Err())
				}
			}
		}
	}

	return &Evaluator{policies: policies, celEnv: env}, nil
}

// Result contains the combined provider selection outcome for a component
// after evaluating all matching policies.
type Result struct {
	// MatchedRules lists the names of rules that matched, in evaluation order.
	MatchedRules []string

	// Required is set if any matching rule mandates a specific provider.
	Required string

	// Preferred lists providers in priority order (from highest-priority rule first).
	Preferred []string

	// Forbidden lists providers that must not be used.
	Forbidden []string

	// Strategy is the provider selection strategy from the highest-priority rule
	// that specified one.
	Strategy string

	// Properties are merged from all matching rules (higher-priority rules override).
	Properties map[string]any
}

// IsProviderAllowed checks whether a given provider name is permitted by this result.
func (r *Result) IsProviderAllowed(name string) bool {
	return !slices.Contains(r.Forbidden, name)
}

// Evaluate checks all policies against a component and returns the combined result.
// Rules are evaluated in priority order (highest first). Within the same priority,
// rules are evaluated in declaration order.
func (e *Evaluator) Evaluate(component *types.Component, app *types.Application) (*Result, error) {
	result := &Result{
		Properties: make(map[string]any),
	}

	// Collect and sort all rules by priority (descending), preserving declaration order within same priority.
	type indexedRule struct {
		rule       types.PolicyRule
		policyName string
		index      int
	}

	var allRules []indexedRule
	for _, policy := range e.policies {
		for i, rule := range policy.Spec.Rules {
			allRules = append(allRules, indexedRule{
				rule:       rule,
				policyName: policy.Metadata.Name,
				index:      i,
			})
		}
	}

	sort.SliceStable(allRules, func(i, j int) bool {
		return allRules[i].rule.Priority > allRules[j].rule.Priority
	})

	appLabels := map[string]string{}
	if app != nil && app.Metadata.Labels != nil {
		appLabels = app.Metadata.Labels
	}
	mergedLabels := mergeLabels(appLabels, component.Labels)

	for _, ir := range allRules {
		matched, err := e.matchesRule(ir.rule, component, mergedLabels, app)
		if err != nil {
			ruleName := ir.rule.Name
			if ruleName == "" {
				ruleName = fmt.Sprintf("rule[%d]", ir.index)
			}
			return nil, fmt.Errorf("policy %q, %s: %w", ir.policyName, ruleName, err)
		}
		if !matched {
			continue
		}

		// Record the matched rule.
		ruleName := ir.rule.Name
		if ruleName == "" {
			ruleName = fmt.Sprintf("%s/rule[%d]", ir.policyName, ir.index)
		}
		result.MatchedRules = append(result.MatchedRules, ruleName)

		// Required: first match wins (highest priority).
		if ir.rule.Providers.Required != "" && result.Required == "" {
			result.Required = ir.rule.Providers.Required
		}

		// Preferred: append in priority order, dedup later.
		result.Preferred = append(result.Preferred, ir.rule.Providers.Preferred...)

		// Forbidden: always accumulate.
		result.Forbidden = append(result.Forbidden, ir.rule.Providers.Forbidden...)

		// Strategy: first match wins.
		if ir.rule.Providers.Strategy != "" && result.Strategy == "" {
			result.Strategy = ir.rule.Providers.Strategy
		}

		// Properties: higher-priority rules set first, lower-priority rules fill gaps.
		for k, v := range ir.rule.Properties {
			if _, exists := result.Properties[k]; !exists {
				result.Properties[k] = v
			}
		}
	}

	// Deduplicate lists while preserving order.
	result.Preferred = dedup(result.Preferred)
	result.Forbidden = dedup(result.Forbidden)

	// Validate: required provider must not be forbidden.
	if result.Required != "" && slices.Contains(result.Forbidden, result.Required) {
		return nil, fmt.Errorf("conflict: provider %q is both required and forbidden for component %q",
			result.Required, component.Name)
	}

	// Remove forbidden providers from preferred list.
	result.Preferred = slices.DeleteFunc(result.Preferred, func(p string) bool {
		return slices.Contains(result.Forbidden, p)
	})

	return result, nil
}

// matchesRule checks if a single rule matches the given component.
func (e *Evaluator) matchesRule(rule types.PolicyRule, component *types.Component, labels map[string]string, app *types.Application) (bool, error) {
	// Type match.
	if rule.Match.Type != "" && rule.Match.Type != component.Type {
		return false, nil
	}

	// Label match: all specified labels must be present.
	for k, v := range rule.Match.Labels {
		if labels[k] != v {
			return false, nil
		}
	}

	// CEL expression match.
	if rule.Match.Expression != "" {
		matched, err := e.evaluateCEL(rule.Match.Expression, component, app)
		if err != nil {
			return false, fmt.Errorf("evaluating expression: %w", err)
		}
		if !matched {
			return false, nil
		}
	}

	return true, nil
}

// evaluateCEL evaluates a CEL expression against component and app context.
func (e *Evaluator) evaluateCEL(expr string, component *types.Component, app *types.Application) (bool, error) {
	ast, issues := e.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return false, issues.Err()
	}

	prog, err := e.celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("creating CEL program: %w", err)
	}

	componentMap := map[string]any{
		"name":   component.Name,
		"type":   component.Type,
		"labels": stringMapToAny(component.Labels),
	}
	if component.Properties != nil {
		componentMap["properties"] = component.Properties
	} else {
		componentMap["properties"] = map[string]any{}
	}

	appMap := map[string]any{
		"name":   "",
		"labels": map[string]any{},
	}
	if app != nil {
		appMap["name"] = app.Metadata.Name
		appMap["labels"] = stringMapToAny(app.Metadata.Labels)
	}

	out, _, err := prog.Eval(map[string]any{
		"component": componentMap,
		"app":       appMap,
	})
	if err != nil {
		return false, fmt.Errorf("evaluating CEL expression: %w", err)
	}

	return isTruthy(out), nil
}

// SelectProvider uses evaluation results to pick the best provider from the
// registry for a given resource type. Selection logic:
//  1. If Required is set, use that provider (error if unavailable).
//  2. Walk the Preferred list, return the first that is registered and supports the type.
//  3. Fall back to any registered provider that supports the type and isn't forbidden.
func SelectProvider(result *Result, registry ProviderRegistry, resourceType types.ResourceType) (types.Provider, error) {
	// 1. Required provider.
	if result.Required != "" {
		p, ok := registry.Get(result.Required)
		if !ok {
			return nil, fmt.Errorf("required provider %q is not registered", result.Required)
		}
		if !providerSupports(p, resourceType) {
			return nil, fmt.Errorf("required provider %q does not support resource type %q", result.Required, resourceType)
		}
		return p, nil
	}

	// 2. Walk preferred list.
	for _, name := range result.Preferred {
		p, ok := registry.Get(name)
		if !ok {
			continue
		}
		if providerSupports(p, resourceType) {
			return p, nil
		}
	}

	// 3. Fall back to any capable, non-forbidden provider.
	candidates := registry.ListProviders()
	for _, p := range candidates {
		if result.IsProviderAllowed(p.Name()) && providerSupports(p, resourceType) {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no available provider for resource type %q after applying policies", resourceType)
}

// ProviderRegistry is the interface the policy engine uses to look up providers.
type ProviderRegistry interface {
	Get(name string) (types.Provider, bool)
	ListProviders() []types.Provider
}

func providerSupports(p types.Provider, rt types.ResourceType) bool {
	return slices.Contains(p.Capabilities(), rt)
}

func mergeLabels(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

func dedup(s []string) []string {
	if len(s) == 0 {
		return s
	}
	seen := make(map[string]bool, len(s))
	result := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func stringMapToAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func isTruthy(val ref.Val) bool {
	v, ok := val.Value().(bool)
	return ok && v
}
