package engine

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

func TestResolveReferences_SimpleString(t *testing.T) {
	state := types.NewState("test")
	state.Resources["server-ip"] = &types.Resource{
		Outputs: map[string]any{"address": "10.0.1.42"},
	}

	props := map[string]any{
		"value": "${server-ip.outputs.address}",
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["value"] != "10.0.1.42" {
		t.Errorf("expected '10.0.1.42', got %v", resolved["value"])
	}
}

func TestResolveReferences_EmbeddedInString(t *testing.T) {
	state := types.NewState("test")
	state.Resources["myvm"] = &types.Resource{
		Outputs: map[string]any{"vmName": "myapp-web-abc123"},
	}

	props := map[string]any{
		"desc": "Attached to ${myvm.outputs.vmName} instance",
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["desc"] != "Attached to myapp-web-abc123 instance" {
		t.Errorf("expected interpolated string, got %v", resolved["desc"])
	}
}

func TestResolveReferences_PreservesType(t *testing.T) {
	state := types.NewState("test")
	state.Resources["db"] = &types.Resource{
		Outputs: map[string]any{"port": 5432},
	}

	props := map[string]any{
		"dbPort": "${db.outputs.port}",
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	// Full-string reference should preserve the original type (int).
	if resolved["dbPort"] != 5432 {
		t.Errorf("expected int 5432, got %v (%T)", resolved["dbPort"], resolved["dbPort"])
	}
}

func TestResolveReferences_NestedMap(t *testing.T) {
	state := types.NewState("test")
	state.Resources["ip"] = &types.Resource{
		Outputs: map[string]any{"address": "10.0.0.1"},
	}

	props := map[string]any{
		"config": map[string]any{
			"target": "${ip.outputs.address}",
		},
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	cfg := resolved["config"].(map[string]any)
	if cfg["target"] != "10.0.0.1" {
		t.Errorf("expected '10.0.0.1', got %v", cfg["target"])
	}
}

func TestResolveReferences_Array(t *testing.T) {
	state := types.NewState("test")
	state.Resources["ip"] = &types.Resource{
		Outputs: map[string]any{"address": "10.0.0.1"},
	}

	props := map[string]any{
		"targets": []any{"${ip.outputs.address}", "static-value"},
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	targets := resolved["targets"].([]any)
	if targets[0] != "10.0.0.1" {
		t.Errorf("expected '10.0.0.1', got %v", targets[0])
	}
	if targets[1] != "static-value" {
		t.Errorf("expected 'static-value', got %v", targets[1])
	}
}

func TestResolveReferences_MissingComponent(t *testing.T) {
	state := types.NewState("test")
	props := map[string]any{
		"value": "${missing.outputs.field}",
	}

	_, err := ResolveReferences(props, state)
	if err == nil {
		t.Fatal("expected error for missing component")
	}
}

func TestResolveReferences_MissingOutput(t *testing.T) {
	state := types.NewState("test")
	state.Resources["server"] = &types.Resource{
		Outputs: map[string]any{"ip": "10.0.0.1"},
	}

	props := map[string]any{
		"value": "${server.outputs.nonexistent}",
	}

	_, err := ResolveReferences(props, state)
	if err == nil {
		t.Fatal("expected error for missing output")
	}
}

func TestResolveReferences_NoReferences(t *testing.T) {
	state := types.NewState("test")
	props := map[string]any{
		"image":    "nginx:latest",
		"replicas": 3,
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["image"] != "nginx:latest" {
		t.Errorf("expected 'nginx:latest', got %v", resolved["image"])
	}
	if resolved["replicas"] != 3 {
		t.Errorf("expected 3, got %v", resolved["replicas"])
	}
}

func TestResolveReferences_MultipleInOneString(t *testing.T) {
	state := types.NewState("test")
	state.Resources["db"] = &types.Resource{
		Outputs: map[string]any{"host": "db.local", "port": 5432},
	}

	props := map[string]any{
		"dsn": "postgres://user:pass@${db.outputs.host}:${db.outputs.port}/mydb",
	}

	resolved, err := ResolveReferences(props, state)
	if err != nil {
		t.Fatal(err)
	}
	expected := "postgres://user:pass@db.local:5432/mydb"
	if resolved["dsn"] != expected {
		t.Errorf("expected %q, got %v", expected, resolved["dsn"])
	}
}

func TestValidateReferences_Valid(t *testing.T) {
	allComponents := map[string]bool{"server": true, "server-ip": true, "server-dns": true}
	props := map[string]any{
		"value": "${server-ip.outputs.address}",
	}

	errs := ValidateReferences(props, "server-dns", []string{"server-ip"}, allComponents)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateReferences_MissingDependsOn(t *testing.T) {
	allComponents := map[string]bool{"server": true, "server-ip": true, "server-dns": true}
	props := map[string]any{
		"value": "${server-ip.outputs.address}",
	}

	errs := ValidateReferences(props, "server-dns", []string{}, allComponents)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateReferences_UnknownComponent(t *testing.T) {
	allComponents := map[string]bool{"server": true}
	props := map[string]any{
		"value": "${nonexistent.outputs.address}",
	}

	errs := ValidateReferences(props, "server-dns", []string{"nonexistent"}, allComponents)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateReferences_NoReferences(t *testing.T) {
	allComponents := map[string]bool{"server": true}
	props := map[string]any{"image": "nginx"}

	errs := ValidateReferences(props, "server", nil, allComponents)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}
