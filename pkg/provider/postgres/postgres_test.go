package postgres

import (
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestProvider() *Provider {
	return NewFromClient(fake.NewSimpleClientset(), "test-ns")
}

func TestProvider_NameAndCapabilities(t *testing.T) {
	p := newTestProvider()

	if p.Name() != "postgres" {
		t.Errorf("expected name 'postgres', got %q", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != types.ResourceTypePostgres {
		t.Errorf("expected [postgres], got %v", caps)
	}
}

func TestProvider_PlanCreate(t *testing.T) {
	p := newTestProvider()

	desired := &types.Resource{
		Name: "mydb",
		Type: types.ResourceTypePostgres,
		Properties: map[string]interface{}{
			"version": "16",
			"storage": "10Gi",
		},
	}

	diff, err := p.Plan(desired, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionCreate {
		t.Errorf("expected create, got %s", diff.Action)
	}
}

func TestProvider_PlanNoChange(t *testing.T) {
	p := newTestProvider()

	res := &types.Resource{
		Name: "mydb",
		Type: types.ResourceTypePostgres,
		Properties: map[string]interface{}{
			"version": "16",
		},
	}

	diff, err := p.Plan(res, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionNone {
		t.Errorf("expected none, got %s", diff.Action)
	}
}

func TestProvider_PlanUpdate(t *testing.T) {
	p := newTestProvider()

	current := &types.Resource{
		Name: "mydb",
		Type: types.ResourceTypePostgres,
		Properties: map[string]interface{}{
			"version": "15",
		},
	}
	desired := &types.Resource{
		Name: "mydb",
		Type: types.ResourceTypePostgres,
		Properties: map[string]interface{}{
			"version": "16",
		},
	}

	diff, err := p.Plan(desired, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.Action != types.DiffActionUpdate {
		t.Errorf("expected update, got %s", diff.Action)
	}
}

func TestProvider_ApplyCreate(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "mydb",
		Type:     types.ResourceTypePostgres,
		Provider: "postgres",
		After: map[string]interface{}{
			"version":        "16",
			"storage":        "5Gi",
			"maxConnections": 200,
			"database":       "petclinic",
			"username":       "admin",
			"password":       "secret",
		},
	}

	res, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if res.Status != types.ResourceStatusReady {
		t.Errorf("expected ready, got %s", res.Status)
	}
	if !strings.HasPrefix(res.Name, "mydb-") {
		t.Errorf("expected name to start with 'mydb-', got %q", res.Name)
	}
	if res.Outputs["database"] != "petclinic" {
		t.Errorf("expected database=petclinic, got %v", res.Outputs["database"])
	}
	if res.Outputs["username"] != "admin" {
		t.Errorf("expected username=admin, got %v", res.Outputs["username"])
	}
	if res.Outputs["namespace"] != "test-ns" {
		t.Errorf("expected namespace=test-ns, got %v", res.Outputs["namespace"])
	}
	connStr, _ := res.Outputs["connectionString"].(string)
	if !strings.Contains(connStr, "petclinic") {
		t.Errorf("expected connectionString to contain db name, got %q", connStr)
	}
	if !strings.Contains(connStr, "admin:secret@") {
		t.Errorf("expected connectionString to contain credentials, got %q", connStr)
	}
}

func TestProvider_ApplyDefaults(t *testing.T) {
	p := newTestProvider()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "db",
		Type:     types.ResourceTypePostgres,
		Provider: "postgres",
		After:    map[string]interface{}{},
	}

	res, err := p.Apply(diff)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if res.Outputs["database"] != "app" {
		t.Errorf("expected default database=app, got %v", res.Outputs["database"])
	}
	if res.Outputs["username"] != "postgres" {
		t.Errorf("expected default username=postgres, got %v", res.Outputs["username"])
	}
	if res.Outputs["port"] != defaultPort {
		t.Errorf("expected default port=%d, got %v", defaultPort, res.Outputs["port"])
	}
}

func TestProvider_ApplyUpdateAndDestroy(t *testing.T) {
	p := newTestProvider()

	// Create first.
	createDiff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "db",
		Type:     types.ResourceTypePostgres,
		Provider: "postgres",
		After: map[string]interface{}{
			"version": "15",
		},
	}
	res, err := p.Apply(createDiff)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Update.
	updateDiff := &types.Diff{
		Action:   types.DiffActionUpdate,
		Resource: res.Name,
		Type:     types.ResourceTypePostgres,
		Provider: "postgres",
		Before:   map[string]interface{}{"version": "15"},
		After:    map[string]interface{}{"version": "16"},
	}
	updated, err := p.Apply(updateDiff)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != res.Name {
		t.Errorf("expected same name after update, got %q vs %q", updated.Name, res.Name)
	}

	// Status should be creating (fake client has 0 ready replicas).
	status, err := p.Status(updated)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status != types.ResourceStatusCreating {
		t.Errorf("expected creating from fake client, got %s", status)
	}

	// Destroy.
	if err := p.Destroy(updated); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	status, err = p.Status(updated)
	if err != nil {
		t.Fatalf("status after destroy: %v", err)
	}
	if status != types.ResourceStatusUnknown {
		t.Errorf("expected unknown after destroy, got %s", status)
	}
}
