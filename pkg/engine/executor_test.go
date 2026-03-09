package engine

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

// fakeProvider simulates a provider for testing, allowing custom output generation.
type fakeProvider struct {
	name         string
	capabilities []types.ResourceType
	outputFn     func(diff *types.Diff) map[string]any
}

func (p *fakeProvider) Name() string                  { return p.name }
func (p *fakeProvider) Capabilities() []types.ResourceType { return p.capabilities }
func (p *fakeProvider) Plan(desired, current *types.Resource) (*types.Diff, error) {
	if current == nil {
		return &types.Diff{
			Action:   types.DiffActionCreate,
			Resource: desired.Name,
			Type:     desired.Type,
			Provider: p.name,
			After:    desired.Properties,
		}, nil
	}
	return &types.Diff{Action: types.DiffActionNone, Resource: current.Name, Type: desired.Type, Provider: p.name}, nil
}
func (p *fakeProvider) Apply(diff *types.Diff) (*types.Resource, error) {
	outputs := map[string]any{"id": diff.Resource}
	if p.outputFn != nil {
		outputs = p.outputFn(diff)
	}
	return &types.Resource{
		Name:       diff.Resource,
		Type:       diff.Type,
		Provider:   p.name,
		Properties: diff.After,
		Outputs:    outputs,
		Status:     types.ResourceStatusReady,
	}, nil
}
func (p *fakeProvider) Destroy(resource *types.Resource) error { return nil }
func (p *fakeProvider) Status(resource *types.Resource) (types.ResourceStatus, error) {
	return types.ResourceStatusReady, nil
}

type fakeRegistry struct {
	providers map[string]types.Provider
}

func (r *fakeRegistry) Get(name string) (types.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}
func (r *fakeRegistry) GetForResource(rt types.ResourceType) (types.Provider, error) {
	for _, p := range r.providers {
		for _, c := range p.Capabilities() {
			if c == rt {
				return p, nil
			}
		}
	}
	return nil, nil
}
func (r *fakeRegistry) ListProviders() []types.Provider {
	var ps []types.Provider
	for _, p := range r.providers {
		ps = append(ps, p)
	}
	return ps
}

func TestExecutor_OutputReferences(t *testing.T) {
	// Simulate the VM -> IP -> DNS flow with output references.
	vmProvider := &fakeProvider{
		name:         "kubevirt",
		capabilities: []types.ResourceType{"vm"},
		outputFn: func(diff *types.Diff) map[string]any {
			return map[string]any{"vmName": "myapp-web-a1b2c3d4"}
		},
	}
	ipProvider := &fakeProvider{
		name:         "static-ipam",
		capabilities: []types.ResourceType{"ip"},
		outputFn: func(diff *types.Diff) map[string]any {
			return map[string]any{
				"address": "10.0.1.42",
				"cidr":    "10.0.1.42/24",
				"pool":    "production",
			}
		},
	}
	dnsProvider := &fakeProvider{
		name:         "mock-dns",
		capabilities: []types.ResourceType{"dns"},
		outputFn: func(diff *types.Diff) map[string]any {
			// Verify the resolved value was passed.
			value, _ := diff.After["value"].(string)
			return map[string]any{
				"fqdn":  diff.After["record"],
				"type":  diff.After["type"],
				"value": value,
			}
		},
	}

	registry := &fakeRegistry{providers: map[string]types.Provider{
		"kubevirt":    vmProvider,
		"static-ipam": ipProvider,
		"mock-dns":    dnsProvider,
	}}

	plan := &Plan{
		AppName: "myapp",
		Steps: []PlanStep{
			{
				Component:   "server",
				Environment: "kubevirt",
				Diff: &types.Diff{
					Action:   types.DiffActionCreate,
					Resource: "myapp-server",
					Type:     "vm",
					Provider: "kubevirt",
					After: map[string]any{
						"image": "fedora:latest",
						"cpu":   4,
					},
				},
			},
			{
				Component:   "server-ip",
				Environment: "static-ipam",
				Diff: &types.Diff{
					Action:   types.DiffActionCreate,
					Resource: "myapp-server-ip",
					Type:     "ip",
					Provider: "static-ipam",
					After: map[string]any{
						"pool":     "production",
						"attachTo": "${server.outputs.vmName}",
					},
				},
			},
			{
				Component:   "server-dns",
				Environment: "mock-dns",
				Diff: &types.Diff{
					Action:   types.DiffActionCreate,
					Resource: "myapp-server-dns",
					Type:     "dns",
					Provider: "mock-dns",
					After: map[string]any{
						"zone":   "example.com",
						"record": "web.example.com",
						"type":   "A",
						"value":  "${server-ip.outputs.address}",
						"ttl":    300,
					},
				},
			},
		},
	}

	state := types.NewState("myapp")
	executor := NewExecutor(registry)
	if err := executor.Execute(plan, state); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// Verify VM was created.
	server := state.Resources["server"]
	if server == nil {
		t.Fatal("server resource not in state")
	}
	if server.Outputs["vmName"] != "myapp-web-a1b2c3d4" {
		t.Errorf("unexpected vmName: %v", server.Outputs["vmName"])
	}

	// Verify IP got the resolved attachTo.
	ip := state.Resources["server-ip"]
	if ip == nil {
		t.Fatal("server-ip resource not in state")
	}
	if ip.Properties["attachTo"] != "myapp-web-a1b2c3d4" {
		t.Errorf("expected attachTo to be resolved, got %v", ip.Properties["attachTo"])
	}
	if ip.Outputs["address"] != "10.0.1.42" {
		t.Errorf("unexpected IP address: %v", ip.Outputs["address"])
	}

	// Verify DNS got the resolved IP value.
	dns := state.Resources["server-dns"]
	if dns == nil {
		t.Fatal("server-dns resource not in state")
	}
	if dns.Properties["value"] != "10.0.1.42" {
		t.Errorf("expected DNS value to be resolved IP, got %v", dns.Properties["value"])
	}
	if dns.Outputs["fqdn"] != "web.example.com" {
		t.Errorf("unexpected fqdn: %v", dns.Outputs["fqdn"])
	}
}

func TestExecutor_UnresolvedReference(t *testing.T) {
	provider := &fakeProvider{
		name:         "mock",
		capabilities: []types.ResourceType{"dns"},
	}
	registry := &fakeRegistry{providers: map[string]types.Provider{"mock": provider}}

	plan := &Plan{
		AppName: "test",
		Steps: []PlanStep{
			{
				Component:   "dns",
				Environment: "mock",
				Diff: &types.Diff{
					Action:   types.DiffActionCreate,
					Resource: "test-dns",
					Type:     "dns",
					Provider: "mock",
					After: map[string]any{
						"value": "${nonexistent.outputs.address}",
					},
				},
			},
		},
	}

	state := types.NewState("test")
	executor := NewExecutor(registry)
	err := executor.Execute(plan, state)
	if err == nil {
		t.Fatal("expected error for unresolved reference")
	}
}
