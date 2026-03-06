package engine

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

func TestNewDAG_ValidComponents(t *testing.T) {
	components := []types.Component{
		{Name: "database", Type: "postgres"},
		{Name: "cache", Type: "redis"},
		{Name: "backend", Type: "container", DependsOn: []string{"database", "cache"}},
		{Name: "frontend", Type: "static-site", DependsOn: []string{"backend"}},
	}

	dag, err := NewDAG(components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dag == nil {
		t.Fatal("expected non-nil DAG")
	}
}

func TestNewDAG_DuplicateNames(t *testing.T) {
	components := []types.Component{
		{Name: "db", Type: "postgres"},
		{Name: "db", Type: "redis"},
	}

	_, err := NewDAG(components)
	if err == nil {
		t.Fatal("expected error for duplicate names")
	}
}

func TestNewDAG_UnknownDependency(t *testing.T) {
	components := []types.Component{
		{Name: "backend", Type: "container", DependsOn: []string{"nonexistent"}},
	}

	_, err := NewDAG(components)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestNewDAG_CycleDetection(t *testing.T) {
	components := []types.Component{
		{Name: "a", Type: "container", DependsOn: []string{"b"}},
		{Name: "b", Type: "container", DependsOn: []string{"c"}},
		{Name: "c", Type: "container", DependsOn: []string{"a"}},
	}

	_, err := NewDAG(components)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
}

func TestDAG_TopologicalSort(t *testing.T) {
	components := []types.Component{
		{Name: "database", Type: "postgres"},
		{Name: "cache", Type: "redis"},
		{Name: "backend", Type: "container", DependsOn: []string{"database", "cache"}},
		{Name: "frontend", Type: "static-site", DependsOn: []string{"backend"}},
	}

	dag, err := NewDAG(components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sorted) != 4 {
		t.Fatalf("expected 4 components, got %d", len(sorted))
	}

	// Build index for position checking.
	pos := make(map[string]int)
	for i, c := range sorted {
		pos[c.Name] = i
	}

	// backend must come after database and cache
	if pos["backend"] < pos["database"] || pos["backend"] < pos["cache"] {
		t.Error("backend should come after database and cache")
	}
	// frontend must come after backend
	if pos["frontend"] < pos["backend"] {
		t.Error("frontend should come after backend")
	}
}

func TestDAG_Levels(t *testing.T) {
	components := []types.Component{
		{Name: "database", Type: "postgres"},
		{Name: "cache", Type: "redis"},
		{Name: "backend", Type: "container", DependsOn: []string{"database", "cache"}},
		{Name: "frontend", Type: "static-site", DependsOn: []string{"backend"}},
	}

	dag, err := NewDAG(components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	levels, err := dag.Levels()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}

	// Level 0: database, cache (no dependencies)
	if len(levels[0]) != 2 {
		t.Errorf("expected 2 components in level 0, got %d", len(levels[0]))
	}
	// Level 1: backend
	if len(levels[1]) != 1 || levels[1][0].Name != "backend" {
		t.Errorf("expected backend in level 1")
	}
	// Level 2: frontend
	if len(levels[2]) != 1 || levels[2][0].Name != "frontend" {
		t.Errorf("expected frontend in level 2")
	}
}
