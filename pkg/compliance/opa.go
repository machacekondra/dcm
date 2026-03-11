package compliance

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Violation represents a single policy violation.
type Violation struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Engine evaluates OPA/Rego policies against deployment inputs.
type Engine struct {
	modules map[string]string // filename -> rego source
}

// NewEngine creates a compliance engine with no policies loaded.
func NewEngine() *Engine {
	return &Engine{modules: make(map[string]string)}
}

// LoadDir loads all .rego files from the given directory.
func (e *Engine) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading policy dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rego") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		e.modules[entry.Name()] = string(data)
		log.Printf("[compliance] loaded policy %s", entry.Name())
	}

	return nil
}

// LoadModule adds a single Rego module by name and source.
func (e *Engine) LoadModule(name, source string) {
	e.modules[name] = source
}

// HasPolicies returns true if any policies are loaded.
func (e *Engine) HasPolicies() bool {
	return len(e.modules) > 0
}

// StepInput is the input passed to OPA for each plan step.
type StepInput struct {
	Component   ComponentInput   `json:"component"`
	Environment EnvironmentInput `json:"environment"`
	Action      string           `json:"action"`
	Application ApplicationInput `json:"application"`
}

// ComponentInput describes the component being deployed.
type ComponentInput struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Labels     map[string]string `json:"labels"`
	Properties map[string]any    `json:"properties"`
	Requires   []string          `json:"requires"`
}

// EnvironmentInput describes the target environment.
type EnvironmentInput struct {
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	Labels       map[string]string `json:"labels"`
	Capabilities []string          `json:"capabilities"`
	Cost         *CostInput        `json:"cost"`
}

// CostInput describes cost metadata.
type CostInput struct {
	Tier       string  `json:"tier"`
	HourlyRate float64 `json:"hourlyRate"`
}

// ApplicationInput describes the application.
type ApplicationInput struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

// Evaluate checks a single step input against all loaded policies.
// Returns violations (deny messages) found.
func (e *Engine) Evaluate(ctx context.Context, input StepInput) ([]Violation, error) {
	if len(e.modules) == 0 {
		return nil, nil
	}

	opts := make([]func(*rego.Rego), 0, len(e.modules)+2)
	opts = append(opts, rego.Query("data.dcm.compliance.deny"))
	for name, src := range e.modules {
		opts = append(opts, rego.Module(name, src))
	}
	opts = append(opts, rego.Input(input))

	rs, err := rego.New(opts...).Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluating policies: %w", err)
	}

	var violations []Violation
	for _, result := range rs {
		for _, expr := range result.Expressions {
			if set, ok := expr.Value.([]interface{}); ok {
				for _, v := range set {
					msg := fmt.Sprintf("%v", v)
					violations = append(violations, Violation{Message: msg})
				}
			}
		}
	}

	return violations, nil
}

// EvaluateAll checks multiple step inputs and returns all violations.
func (e *Engine) EvaluateAll(ctx context.Context, inputs []StepInput) ([]Violation, error) {
	var all []Violation
	for _, input := range inputs {
		violations, err := e.Evaluate(ctx, input)
		if err != nil {
			return nil, err
		}
		all = append(all, violations...)
	}
	return all, nil
}
