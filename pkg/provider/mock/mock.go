package mock

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/types"
)

// Provider is a mock provider for local testing.
// It simulates resource creation without actually provisioning anything.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "mock"
}

func (p *Provider) Capabilities() []types.ResourceType {
	return []types.ResourceType{
		types.ResourceTypeContainer,
		types.ResourceTypePostgres,
		types.ResourceTypeRedis,
		types.ResourceTypeStaticSite,
		types.ResourceTypeNetwork,
		types.ResourceTypeStorage,
	}
}

func (p *Provider) Plan(desired, current *types.Resource) (*types.Diff, error) {
	if current == nil {
		return &types.Diff{
			Action:   types.DiffActionCreate,
			Resource: desired.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			After:    desired.Properties,
		}, nil
	}

	// Simple change detection: compare properties.
	if !propsEqual(desired.Properties, current.Properties) {
		return &types.Diff{
			Action:   types.DiffActionUpdate,
			Resource: current.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			Before:   current.Properties,
			After:    desired.Properties,
		}, nil
	}

	return &types.Diff{
		Action:   types.DiffActionNone,
		Resource: current.Name,
		Type:     desired.Type,
		Provider: p.Name(),
	}, nil
}

func (p *Provider) Apply(diff *types.Diff) (*types.Resource, error) {
	return &types.Resource{
		Name:       diff.Resource,
		Type:       diff.Type,
		Provider:   p.Name(),
		Properties: diff.After,
		Outputs:    generateMockOutputs(diff.Type, diff.Resource),
		Status:     types.ResourceStatusReady,
	}, nil
}

func (p *Provider) Destroy(resource *types.Resource) error {
	return nil
}

func (p *Provider) Status(resource *types.Resource) (types.ResourceStatus, error) {
	return types.ResourceStatusReady, nil
}

func generateMockOutputs(rt types.ResourceType, name string) map[string]interface{} {
	switch rt {
	case types.ResourceTypePostgres:
		return map[string]interface{}{
			"connectionString": fmt.Sprintf("postgres://user:pass@%s-mock:5432/%s", name, name),
			"host":             fmt.Sprintf("%s-mock", name),
			"port":             5432,
		}
	case types.ResourceTypeRedis:
		return map[string]interface{}{
			"endpoint": fmt.Sprintf("redis://%s-mock:6379", name),
			"host":     fmt.Sprintf("%s-mock", name),
			"port":     6379,
		}
	case types.ResourceTypeContainer:
		return map[string]interface{}{
			"url":        fmt.Sprintf("http://%s-mock.local:8080", name),
			"internalIP": "10.0.0.1",
		}
	case types.ResourceTypeStaticSite:
		return map[string]interface{}{
			"url": fmt.Sprintf("https://%s-mock.local", name),
			"cdn": fmt.Sprintf("https://cdn.mock.local/%s", name),
		}
	default:
		return map[string]interface{}{
			"id": fmt.Sprintf("mock-%s", name),
		}
	}
}

func propsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if fmt.Sprintf("%v", b[k]) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}
