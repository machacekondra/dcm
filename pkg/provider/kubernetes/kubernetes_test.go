package kubernetes

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestProvider() *Provider {
	return NewFromClient(fake.NewSimpleClientset(), "test-ns")
}

func TestProvider_NameAndCapabilities(t *testing.T) {
	p := newTestProvider()

	if p.Name() != "kubernetes" {
		t.Errorf("expected name 'kubernetes', got %q", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != types.ResourceTypeContainer {
		t.Errorf("expected [container], got %v", caps)
	}
}

func TestProvider_PlanCreate(t *testing.T) {
	p := newTestProvider()

	desired := &types.Resource{
		Name: "web",
		Type: types.ResourceTypeContainer,
		Properties: map[string]interface{}{
			"image":    "nginx:latest",
			"replicas": 2,
		},
	}

	diff, err := p.Plan(desired, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionCreate {
		t.Errorf("expected create action, got %s", diff.Action)
	}
}

func TestProvider_PlanNoChange(t *testing.T) {
	p := newTestProvider()

	resource := &types.Resource{
		Name: "web",
		Type: types.ResourceTypeContainer,
		Properties: map[string]interface{}{
			"image": "nginx:latest",
		},
	}

	diff, err := p.Plan(resource, resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionNone {
		t.Errorf("expected no-op, got %s", diff.Action)
	}
}

func TestProvider_ApplyAndDestroy(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After: map[string]interface{}{
			"image":    "nginx:latest",
			"replicas": 3,
			"port":     8080,
			"env": map[string]interface{}{
				"APP_ENV": "production",
			},
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if resource.Status != types.ResourceStatusReady {
		t.Errorf("expected ready status, got %s", resource.Status)
	}
	if resource.Outputs["namespace"] != "test-ns" {
		t.Errorf("expected namespace test-ns in outputs, got %v", resource.Outputs["namespace"])
	}

	// Verify status reports correctly (fake client won't have ready replicas).
	status, err := p.Status(resource)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status != types.ResourceStatusCreating {
		t.Errorf("expected creating status from fake client, got %s", status)
	}

	// Destroy should succeed.
	if err := p.Destroy(resource); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	// Status after destroy should be unknown.
	status, err = p.Status(resource)
	if err != nil {
		t.Fatalf("status after destroy failed: %v", err)
	}
	if status != types.ResourceStatusUnknown {
		t.Errorf("expected unknown status after destroy, got %s", status)
	}
}

func TestProvider_ApplyMissingImage(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "web",
		Type:     types.ResourceTypeContainer,
		Provider: "kubernetes",
		After:    map[string]interface{}{},
	}

	_, err := p.Apply(diff)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}
