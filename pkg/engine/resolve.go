package engine

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dcm-io/dcm/pkg/types"
)

// refPattern matches output references like ${component.outputs.field}.
var refPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_-]+)\.outputs\.([a-zA-Z0-9_.-]+)\}`)

// ResolveReferences replaces ${component.outputs.field} references in properties
// with actual values from the state. It returns a new map with resolved values.
func ResolveReferences(props map[string]any, state *types.State) (map[string]any, error) {
	if props == nil {
		return nil, nil
	}
	resolved := make(map[string]any, len(props))
	for k, v := range props {
		rv, err := resolveValue(v, state)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", k, err)
		}
		resolved[k] = rv
	}
	return resolved, nil
}

func resolveValue(v any, state *types.State) (any, error) {
	switch val := v.(type) {
	case string:
		return resolveString(val, state)
	case map[string]any:
		resolved := make(map[string]any, len(val))
		for k, inner := range val {
			rv, err := resolveValue(inner, state)
			if err != nil {
				return nil, err
			}
			resolved[k] = rv
		}
		return resolved, nil
	case []any:
		resolved := make([]any, len(val))
		for i, inner := range val {
			rv, err := resolveValue(inner, state)
			if err != nil {
				return nil, err
			}
			resolved[i] = rv
		}
		return resolved, nil
	default:
		return v, nil
	}
}

func resolveString(s string, state *types.State) (any, error) {
	// If the entire string is a single reference, return the raw output value
	// (preserving its type — could be a number, bool, etc.).
	if match := refPattern.FindStringSubmatch(s); match != nil && match[0] == s {
		return lookupOutput(match[1], match[2], state)
	}

	// Otherwise, do string interpolation for embedded references.
	var resolveErr error
	result := refPattern.ReplaceAllStringFunc(s, func(ref string) string {
		match := refPattern.FindStringSubmatch(ref)
		val, err := lookupOutput(match[1], match[2], state)
		if err != nil {
			resolveErr = err
			return ref
		}
		return fmt.Sprintf("%v", val)
	})
	if resolveErr != nil {
		return nil, resolveErr
	}
	return result, nil
}

func lookupOutput(component, field string, state *types.State) (any, error) {
	resource, ok := state.Resources[component]
	if !ok {
		return nil, fmt.Errorf("referenced component %q not found in state", component)
	}
	if resource.Outputs == nil {
		return nil, fmt.Errorf("component %q has no outputs", component)
	}
	val, ok := resource.Outputs[field]
	if !ok {
		return nil, fmt.Errorf("component %q has no output %q (available: %s)",
			component, field, outputKeys(resource.Outputs))
	}
	return val, nil
}

func outputKeys(outputs map[string]any) string {
	keys := make([]string, 0, len(outputs))
	for k := range outputs {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// ValidateReferences checks that all ${component.outputs.field} references
// in properties point to components that exist and are listed in dependsOn.
func ValidateReferences(props map[string]any, componentName string, deps []string, allComponents map[string]bool) []error {
	refs := extractReferences(props)
	if len(refs) == 0 {
		return nil
	}

	depSet := make(map[string]bool, len(deps))
	for _, d := range deps {
		depSet[d] = true
	}

	var errs []error
	for _, ref := range refs {
		if !allComponents[ref] {
			errs = append(errs, fmt.Errorf("component %q references unknown component %q", componentName, ref))
		} else if !depSet[ref] {
			errs = append(errs, fmt.Errorf("component %q references %q but does not list it in dependsOn", componentName, ref))
		}
	}
	return errs
}

// extractReferences returns all component names referenced via ${...} in properties.
func extractReferences(props map[string]any) []string {
	seen := make(map[string]bool)
	extractFromValue(props, seen)
	refs := make([]string, 0, len(seen))
	for r := range seen {
		refs = append(refs, r)
	}
	return refs
}

func extractFromValue(v any, seen map[string]bool) {
	switch val := v.(type) {
	case string:
		for _, match := range refPattern.FindAllStringSubmatch(val, -1) {
			seen[match[1]] = true
		}
	case map[string]any:
		for _, inner := range val {
			extractFromValue(inner, seen)
		}
	case []any:
		for _, inner := range val {
			extractFromValue(inner, seen)
		}
	}
}
