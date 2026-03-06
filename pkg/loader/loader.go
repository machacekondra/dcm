package loader

import (
	"fmt"
	"os"

	"github.com/dcm-io/dcm/pkg/types"
	"gopkg.in/yaml.v3"
)

// LoadApplication reads and parses an application YAML file.
func LoadApplication(path string) (*types.Application, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading application file %s: %w", path, err)
	}

	var app types.Application
	if err := yaml.Unmarshal(data, &app); err != nil {
		return nil, fmt.Errorf("parsing application file %s: %w", path, err)
	}

	if app.Kind != "Application" {
		return nil, fmt.Errorf("expected kind Application, got %q", app.Kind)
	}

	return &app, nil
}

// LoadPolicy reads and parses a policy YAML file.
func LoadPolicy(path string) (*types.Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file %s: %w", path, err)
	}

	var policy types.Policy
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("parsing policy file %s: %w", path, err)
	}

	if policy.Kind != "Policy" {
		return nil, fmt.Errorf("expected kind Policy, got %q", policy.Kind)
	}

	return &policy, nil
}
