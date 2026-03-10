package engine

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/types"
)

// PlacementGroup is a set of components that must be scheduled to the same environment.
type PlacementGroup struct {
	Components   []*types.Component
	Requirements []string // union of all members' Requires
}

// BuildPlacementGroups constructs placement groups from colocateWith links.
// Components linked (directly or transitively) via colocateWith form one group.
// Components with no colocateWith form singleton groups.
func BuildPlacementGroups(components []*types.Component) ([]PlacementGroup, error) {
	byName := make(map[string]*types.Component, len(components))
	for _, c := range components {
		byName[c.Name] = c
	}

	// Validate colocateWith targets exist.
	for _, c := range components {
		if c.ColocateWith != "" {
			if _, ok := byName[c.ColocateWith]; !ok {
				return nil, fmt.Errorf("component %q: colocateWith target %q not found", c.Name, c.ColocateWith)
			}
		}
	}

	// Union-Find to group components.
	parent := make(map[string]string, len(components))
	for _, c := range components {
		parent[c.Name] = c.Name
	}

	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}

	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for _, c := range components {
		if c.ColocateWith != "" {
			union(c.Name, c.ColocateWith)
		}
	}

	// Collect groups by root.
	groups := make(map[string]*PlacementGroup)
	for _, c := range components {
		root := find(c.Name)
		if _, ok := groups[root]; !ok {
			groups[root] = &PlacementGroup{}
		}
		g := groups[root]
		g.Components = append(g.Components, c)
		for _, req := range c.Requires {
			if !contains(g.Requirements, req) {
				g.Requirements = append(g.Requirements, req)
			}
		}
	}

	// Return in deterministic order (same as component order, by first member).
	seen := make(map[string]bool)
	var result []PlacementGroup
	for _, c := range components {
		root := find(c.Name)
		if seen[root] {
			continue
		}
		seen[root] = true
		result = append(result, *groups[root])
	}

	return result, nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
