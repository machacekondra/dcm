package compliance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const testPolicy = `
package dcm.compliance

deny contains msg if {
	input.component.type == "postgres"
	input.environment.labels.env == "prod"
	storage := input.component.properties.storage
	not startswith(storage, "10")
	not startswith(storage, "20")
	not startswith(storage, "50")
	not startswith(storage, "100")
	msg := sprintf("postgres %q in prod requires at least 10Gi storage, got %s", [input.component.name, storage])
}

deny contains msg if {
	input.component.type == "container"
	not input.component.properties.replicas
	input.environment.labels.env == "prod"
	msg := sprintf("container %q in prod must specify replicas", [input.component.name])
}

deny contains msg if {
	input.environment.cost.hourlyRate > 1.0
	msg := sprintf("environment %q exceeds cost limit ($%.2f/hr)", [input.environment.name, input.environment.cost.hourlyRate])
}
`

func newTestEngine() *Engine {
	e := NewEngine()
	e.LoadModule("test.rego", testPolicy)
	return e
}

func TestEvaluate_NoViolations(t *testing.T) {
	e := newTestEngine()
	input := StepInput{
		Component: ComponentInput{
			Name: "db",
			Type: "postgres",
			Properties: map[string]any{
				"storage": "20Gi",
			},
		},
		Environment: EnvironmentInput{
			Name:     "prod-cluster",
			Provider: "kubernetes",
			Labels:   map[string]string{"env": "prod"},
			Cost:     &CostInput{Tier: "standard", HourlyRate: 0.10},
		},
		Application: ApplicationInput{Name: "myapp"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %v", violations)
	}
}

func TestEvaluate_StorageViolation(t *testing.T) {
	e := newTestEngine()
	input := StepInput{
		Component: ComponentInput{
			Name: "db",
			Type: "postgres",
			Properties: map[string]any{
				"storage": "1Gi",
			},
		},
		Environment: EnvironmentInput{
			Name:   "prod-cluster",
			Labels: map[string]string{"env": "prod"},
			Cost:   &CostInput{Tier: "standard", HourlyRate: 0.05},
		},
		Application: ApplicationInput{Name: "myapp"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Message == "" {
		t.Error("expected non-empty violation message")
	}
}

func TestEvaluate_ReplicasViolation(t *testing.T) {
	e := newTestEngine()
	input := StepInput{
		Component: ComponentInput{
			Name:       "app",
			Type:       "container",
			Properties: map[string]any{"image": "nginx"},
		},
		Environment: EnvironmentInput{
			Name:   "prod-cluster",
			Labels: map[string]string{"env": "prod"},
			Cost:   &CostInput{Tier: "standard", HourlyRate: 0.05},
		},
		Application: ApplicationInput{Name: "myapp"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_CostViolation(t *testing.T) {
	e := newTestEngine()
	input := StepInput{
		Component: ComponentInput{
			Name:       "app",
			Type:       "container",
			Properties: map[string]any{"image": "nginx", "replicas": 2},
		},
		Environment: EnvironmentInput{
			Name:   "expensive-env",
			Labels: map[string]string{"env": "dev"},
			Cost:   &CostInput{Tier: "premium", HourlyRate: 5.0},
		},
		Application: ApplicationInput{Name: "myapp"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 cost violation, got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_MultipleViolations(t *testing.T) {
	e := newTestEngine()
	input := StepInput{
		Component: ComponentInput{
			Name:       "app",
			Type:       "container",
			Properties: map[string]any{"image": "nginx"},
		},
		Environment: EnvironmentInput{
			Name:   "prod-cluster",
			Labels: map[string]string{"env": "prod"},
			Cost:   &CostInput{Tier: "premium", HourlyRate: 2.0},
		},
		Application: ApplicationInput{Name: "myapp"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations (replicas + cost), got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_NoPolicies(t *testing.T) {
	e := NewEngine()
	input := StepInput{
		Component:   ComponentInput{Name: "x", Type: "container"},
		Environment: EnvironmentInput{Name: "dev"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations with no policies, got %v", violations)
	}
}

func TestEvaluate_DevEnvironmentNoRestrictions(t *testing.T) {
	e := newTestEngine()
	input := StepInput{
		Component: ComponentInput{
			Name:       "db",
			Type:       "postgres",
			Properties: map[string]any{"storage": "1Gi"},
		},
		Environment: EnvironmentInput{
			Name:   "dev-cluster",
			Labels: map[string]string{"env": "dev"},
			Cost:   &CostInput{Tier: "standard", HourlyRate: 0.05},
		},
		Application: ApplicationInput{Name: "myapp"},
	}

	violations, err := e.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations in dev, got %v", violations)
	}
}

func TestEvaluateAll(t *testing.T) {
	e := newTestEngine()
	inputs := []StepInput{
		{
			Component: ComponentInput{
				Name: "db", Type: "postgres",
				Properties: map[string]any{"storage": "1Gi"},
			},
			Environment: EnvironmentInput{
				Name: "prod", Labels: map[string]string{"env": "prod"},
				Cost: &CostInput{HourlyRate: 0.05},
			},
		},
		{
			Component: ComponentInput{
				Name: "app", Type: "container",
				Properties: map[string]any{"image": "nginx"},
			},
			Environment: EnvironmentInput{
				Name: "prod", Labels: map[string]string{"env": "prod"},
				Cost: &CostInput{HourlyRate: 0.05},
			},
		},
	}

	violations, err := e.EvaluateAll(context.Background(), inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations (storage + replicas), got %d: %v", len(violations), violations)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "policy.rego"), []byte(testPolicy), 0644)
	os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("not rego"), 0644)

	e := NewEngine()
	if err := e.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if !e.HasPolicies() {
		t.Error("expected policies to be loaded")
	}
	if len(e.modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(e.modules))
	}
}

func TestLoadDir_NotExist(t *testing.T) {
	e := NewEngine()
	if err := e.LoadDir("/nonexistent/path"); err != nil {
		t.Errorf("expected no error for missing dir, got: %v", err)
	}
	if e.HasPolicies() {
		t.Error("expected no policies loaded")
	}
}
