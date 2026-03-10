package engine

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

func ptrComponents(cs ...types.Component) []*types.Component {
	result := make([]*types.Component, len(cs))
	for i := range cs {
		result[i] = &cs[i]
	}
	return result
}

func TestBuildPlacementGroups_Singleton(t *testing.T) {
	components := ptrComponents(
		types.Component{Name: "app", Type: "container"},
		types.Component{Name: "db", Type: "postgres"},
	)

	groups, err := BuildPlacementGroups(components)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0].Components) != 1 || groups[0].Components[0].Name != "app" {
		t.Errorf("expected group 0 to contain 'app'")
	}
	if len(groups[1].Components) != 1 || groups[1].Components[0].Name != "db" {
		t.Errorf("expected group 1 to contain 'db'")
	}
}

func TestBuildPlacementGroups_Colocated(t *testing.T) {
	components := ptrComponents(
		types.Component{Name: "app", Type: "container", Requires: []string{"loadbalancer"}},
		types.Component{Name: "app-ip", Type: "ip", ColocateWith: "app", Requires: []string{"loadbalancer"}},
		types.Component{Name: "dns", Type: "dns"},
	)

	groups, err := BuildPlacementGroups(components)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// First group: app + app-ip.
	if len(groups[0].Components) != 2 {
		t.Fatalf("expected group 0 to have 2 components, got %d", len(groups[0].Components))
	}
	if len(groups[0].Requirements) != 1 || groups[0].Requirements[0] != "loadbalancer" {
		t.Errorf("expected merged requirements [loadbalancer], got %v", groups[0].Requirements)
	}

	// Second group: dns (singleton).
	if len(groups[1].Components) != 1 || groups[1].Components[0].Name != "dns" {
		t.Errorf("expected group 1 to contain 'dns'")
	}
}

func TestBuildPlacementGroups_Transitive(t *testing.T) {
	components := ptrComponents(
		types.Component{Name: "app", Type: "container", Requires: []string{"loadbalancer"}},
		types.Component{Name: "db", Type: "postgres", ColocateWith: "app", Requires: []string{"persistent-storage"}},
		types.Component{Name: "cache", Type: "redis", ColocateWith: "db"},
	)

	groups, err := BuildPlacementGroups(components)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group (transitive), got %d", len(groups))
	}
	if len(groups[0].Components) != 3 {
		t.Fatalf("expected 3 components in group, got %d", len(groups[0].Components))
	}
	if len(groups[0].Requirements) != 2 {
		t.Errorf("expected 2 merged requirements, got %v", groups[0].Requirements)
	}
}

func TestBuildPlacementGroups_InvalidTarget(t *testing.T) {
	components := ptrComponents(
		types.Component{Name: "app", Type: "container", ColocateWith: "nonexistent"},
	)

	_, err := BuildPlacementGroups(components)
	if err == nil {
		t.Fatal("expected error for invalid colocateWith target")
	}
}

func TestBuildPlacementGroups_MergedRequirements(t *testing.T) {
	components := ptrComponents(
		types.Component{Name: "app", Type: "container", Requires: []string{"loadbalancer", "gpu"}},
		types.Component{Name: "ip", Type: "ip", ColocateWith: "app", Requires: []string{"loadbalancer"}},
	)

	groups, err := BuildPlacementGroups(components)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	// Requirements should be deduplicated: [loadbalancer, gpu].
	reqs := groups[0].Requirements
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requirements, got %v", reqs)
	}
}
