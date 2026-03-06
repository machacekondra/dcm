package engine

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/types"
)

// DAG represents a directed acyclic graph of components.
type DAG struct {
	nodes map[string]*types.Component
	edges map[string][]string // node -> dependencies
}

// NewDAG builds a DAG from an application's components.
func NewDAG(components []types.Component) (*DAG, error) {
	dag := &DAG{
		nodes: make(map[string]*types.Component),
		edges: make(map[string][]string),
	}

	for i := range components {
		c := &components[i]
		if _, exists := dag.nodes[c.Name]; exists {
			return nil, fmt.Errorf("duplicate component name: %s", c.Name)
		}
		dag.nodes[c.Name] = c
		dag.edges[c.Name] = c.DependsOn
	}

	// Validate that all dependencies reference existing components.
	for name, deps := range dag.edges {
		for _, dep := range deps {
			if _, exists := dag.nodes[dep]; !exists {
				return nil, fmt.Errorf("component %q depends on unknown component %q", name, dep)
			}
		}
	}

	if cycle := dag.detectCycle(); cycle != nil {
		return nil, fmt.Errorf("dependency cycle detected: %v", cycle)
	}

	return dag, nil
}

// TopologicalSort returns components in dependency order (dependencies first).
func (d *DAG) TopologicalSort() ([]*types.Component, error) {
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var sorted []*types.Component

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("cycle detected at %q", name)
		}
		visiting[name] = true

		for _, dep := range d.edges[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		sorted = append(sorted, d.nodes[name])
		return nil
	}

	for name := range d.nodes {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}

// Levels returns components grouped by execution level.
// Components within the same level have no dependencies on each other
// and can be executed in parallel.
func (d *DAG) Levels() ([][]*types.Component, error) {
	inDegree := make(map[string]int)
	for name := range d.nodes {
		inDegree[name] = 0
	}
	for _, deps := range d.edges {
		for _, dep := range deps {
			_ = dep // edges point to dependencies, not dependents
		}
	}
	// Build reverse: for each node, count how many times it appears as a dependency target
	dependents := make(map[string][]string) // dep -> nodes that depend on it
	for name, deps := range d.edges {
		inDegree[name] += len(deps)
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var levels [][]*types.Component
	ready := make([]string, 0)

	for name, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, name)
		}
	}

	for len(ready) > 0 {
		level := make([]*types.Component, 0, len(ready))
		var nextReady []string

		for _, name := range ready {
			level = append(level, d.nodes[name])
			for _, dependent := range dependents[name] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextReady = append(nextReady, dependent)
				}
			}
		}

		levels = append(levels, level)
		ready = nextReady
	}

	// Check for unprocessed nodes (would indicate a cycle, but we already check above).
	processed := 0
	for _, level := range levels {
		processed += len(level)
	}
	if processed != len(d.nodes) {
		return nil, fmt.Errorf("not all components were processed; possible cycle")
	}

	return levels, nil
}

// GetComponent returns a component by name.
func (d *DAG) GetComponent(name string) (*types.Component, bool) {
	c, ok := d.nodes[name]
	return c, ok
}

// detectCycle uses DFS to find cycles.
func (d *DAG) detectCycle() []string {
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var path []string

	var dfs func(name string) bool
	dfs = func(name string) bool {
		if visited[name] {
			return false
		}
		if visiting[name] {
			path = append(path, name)
			return true
		}
		visiting[name] = true
		path = append(path, name)

		for _, dep := range d.edges[name] {
			if dfs(dep) {
				return true
			}
		}

		path = path[:len(path)-1]
		visiting[name] = false
		visited[name] = true
		return false
	}

	for name := range d.nodes {
		if dfs(name) {
			return path
		}
	}
	return nil
}
