package policy

import (
	"github.com/dcm-io/dcm/pkg/types"
)

// Evaluator evaluates policies against components to determine provider selection.
type Evaluator struct {
	policies []types.Policy
}

// NewEvaluator creates a policy evaluator with the given policies.
func NewEvaluator(policies []types.Policy) *Evaluator {
	return &Evaluator{policies: policies}
}

// Result contains the provider selection outcome for a component.
type Result struct {
	Preferred  []string
	Forbidden  []string
	Strategy   string
	Properties map[string]interface{}
}

// Evaluate checks all policies against a component and returns the combined result.
func (e *Evaluator) Evaluate(component *types.Component, appLabels map[string]string) *Result {
	result := &Result{
		Properties: make(map[string]interface{}),
	}

	mergedLabels := mergeLabels(appLabels, component.Labels)

	for _, policy := range e.policies {
		for _, rule := range policy.Spec.Rules {
			if matchesRule(rule, component, mergedLabels) {
				result.Preferred = append(result.Preferred, rule.Providers.Preferred...)
				result.Forbidden = append(result.Forbidden, rule.Providers.Forbidden...)
				if rule.Providers.Strategy != "" {
					result.Strategy = rule.Providers.Strategy
				}
				for k, v := range rule.Properties {
					result.Properties[k] = v
				}
			}
		}
	}

	return result
}

func matchesRule(rule types.PolicyRule, component *types.Component, labels map[string]string) bool {
	// If a type is specified, it must match.
	if rule.Match.Type != "" && rule.Match.Type != component.Type {
		return false
	}

	// All match labels must be present on the component.
	for k, v := range rule.Match.Labels {
		if labels[k] != v {
			return false
		}
	}

	return true
}

func mergeLabels(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}
