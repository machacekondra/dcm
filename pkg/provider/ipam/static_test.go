package ipam

import (
	"fmt"
	"net"
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

func newTestProvider(t *testing.T) *StaticProvider {
	t.Helper()
	p, err := NewStatic(StaticConfig{
		Pools: map[string]string{
			"production": "10.0.1.0/24",
			"staging":    "192.168.1.0/24",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStaticProvider_Name(t *testing.T) {
	p := newTestProvider(t)
	if p.Name() != "static-ipam" {
		t.Errorf("expected 'static-ipam', got %q", p.Name())
	}
}

func TestStaticProvider_Capabilities(t *testing.T) {
	p := newTestProvider(t)
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != types.ResourceTypeIP {
		t.Errorf("expected [ip], got %v", caps)
	}
}

func TestStaticProvider_AllocateAndDestroy(t *testing.T) {
	p := newTestProvider(t)

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "myapp-server-ip",
		Type:     types.ResourceTypeIP,
		Provider: "static-ipam",
		After:    map[string]any{"pool": "production"},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	// Verify outputs.
	addr, ok := resource.Outputs["address"].(string)
	if !ok || addr == "" {
		t.Fatal("expected non-empty address output")
	}

	// Verify allocated IP is within the subnet.
	ip := net.ParseIP(addr)
	if ip == nil {
		t.Fatalf("invalid IP: %s", addr)
	}
	_, subnet, _ := net.ParseCIDR("10.0.1.0/24")
	if !subnet.Contains(ip) {
		t.Errorf("IP %s not in subnet %s", addr, subnet)
	}

	if resource.Outputs["pool"] != "production" {
		t.Errorf("expected pool 'production', got %v", resource.Outputs["pool"])
	}

	cidr, ok := resource.Outputs["cidr"].(string)
	if !ok || cidr == "" {
		t.Fatal("expected non-empty cidr output")
	}

	// Verify status.
	status, err := p.Status(resource)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.ResourceStatusReady {
		t.Errorf("expected ready, got %s", status)
	}

	// Destroy and verify status.
	if err := p.Destroy(resource); err != nil {
		t.Fatal(err)
	}

	status, err = p.Status(resource)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.ResourceStatusUnknown {
		t.Errorf("expected unknown after destroy, got %s", status)
	}
}

func TestStaticProvider_UniqueAllocations(t *testing.T) {
	p := newTestProvider(t)

	allocated := make(map[string]bool)
	for i := range 10 {
		diff := &types.Diff{
			Action:   types.DiffActionCreate,
			Resource: fmt.Sprintf("resource-%d", i),
			Type:     types.ResourceTypeIP,
			Provider: "static-ipam",
			After:    map[string]any{"pool": "production"},
		}
		resource, err := p.Apply(diff)
		if err != nil {
			t.Fatal(err)
		}
		addr := resource.Outputs["address"].(string)
		if allocated[addr] {
			t.Errorf("duplicate IP allocated: %s", addr)
		}
		allocated[addr] = true
	}
}

func TestStaticProvider_UnknownPool(t *testing.T) {
	p := newTestProvider(t)

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "test",
		Type:     types.ResourceTypeIP,
		Provider: "static-ipam",
		After:    map[string]any{"pool": "nonexistent"},
	}

	_, err := p.Apply(diff)
	if err == nil {
		t.Fatal("expected error for unknown pool")
	}
}

func TestStaticProvider_MissingPool(t *testing.T) {
	p := newTestProvider(t)

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "test",
		Type:     types.ResourceTypeIP,
		Provider: "static-ipam",
		After:    map[string]any{},
	}

	_, err := p.Apply(diff)
	if err == nil {
		t.Fatal("expected error for missing pool")
	}
}

func TestStaticProvider_PlanCreate(t *testing.T) {
	p := newTestProvider(t)

	desired := &types.Resource{
		Name:       "myapp-ip",
		Type:       types.ResourceTypeIP,
		Properties: map[string]any{"pool": "production"},
	}

	diff, err := p.Plan(desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionCreate {
		t.Errorf("expected create, got %s", diff.Action)
	}
}

func TestStaticProvider_PlanNoChange(t *testing.T) {
	p := newTestProvider(t)

	resource := &types.Resource{
		Name:       "myapp-ip",
		Type:       types.ResourceTypeIP,
		Properties: map[string]any{"pool": "production"},
	}

	diff, err := p.Plan(resource, resource)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionNone {
		t.Errorf("expected none, got %s", diff.Action)
	}
}

func TestStaticProvider_InvalidCIDR(t *testing.T) {
	_, err := NewStatic(StaticConfig{
		Pools: map[string]string{"bad": "not-a-cidr"},
	})
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

