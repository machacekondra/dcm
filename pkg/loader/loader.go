package loader

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

// LoadPolicy reads and parses a single policy YAML file.
// Supports multi-document YAML (multiple policies separated by ---).
func LoadPolicy(path string) ([]types.Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file %s: %w", path, err)
	}

	return parsePolicies(data, path)
}

// LoadPolicies loads policies from one or more paths. Each path can be:
//   - A single YAML file
//   - A directory (all .yaml/.yml files are loaded recursively)
func LoadPolicies(paths []string) ([]types.Policy, error) {
	var all []types.Policy

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			dirPolicies, err := loadPoliciesFromDir(path)
			if err != nil {
				return nil, err
			}
			all = append(all, dirPolicies...)
		} else {
			filePolicies, err := LoadPolicy(path)
			if err != nil {
				return nil, err
			}
			all = append(all, filePolicies...)
		}
	}

	return all, nil
}

// LoadEnvironment reads and parses a single environment YAML file.
// Supports multi-document YAML (multiple environments separated by ---).
func LoadEnvironment(path string) ([]types.Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading environment file %s: %w", path, err)
	}

	return parseEnvironments(data, path)
}

// LoadEnvironments loads environments from one or more paths. Each path can be:
//   - A single YAML file
//   - A directory (all .yaml/.yml files are loaded recursively)
func LoadEnvironments(paths []string) ([]types.Environment, error) {
	var all []types.Environment

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			dirEnvs, err := loadEnvironmentsFromDir(path)
			if err != nil {
				return nil, err
			}
			all = append(all, dirEnvs...)
		} else {
			fileEnvs, err := LoadEnvironment(path)
			if err != nil {
				return nil, err
			}
			all = append(all, fileEnvs...)
		}
	}

	return all, nil
}

func loadPoliciesFromDir(dir string) ([]types.Policy, error) {
	var all []types.Policy

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		policies, err := LoadPolicy(path)
		if err != nil {
			return err
		}
		all = append(all, policies...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking policy directory %s: %w", dir, err)
	}

	return all, nil
}

func parsePolicies(data []byte, source string) ([]types.Policy, error) {
	var policies []types.Policy
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var policy types.Policy
		err := decoder.Decode(&policy)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing policy in %s: %w", source, err)
		}
		if policy.Kind != "Policy" {
			return nil, fmt.Errorf("expected kind Policy in %s, got %q", source, policy.Kind)
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

func loadEnvironmentsFromDir(dir string) ([]types.Environment, error) {
	var all []types.Environment

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		envs, err := LoadEnvironment(path)
		if err != nil {
			return err
		}
		all = append(all, envs...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking environment directory %s: %w", dir, err)
	}

	return all, nil
}

func parseEnvironments(data []byte, source string) ([]types.Environment, error) {
	var envs []types.Environment
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var env types.Environment
		err := decoder.Decode(&env)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing environment in %s: %w", source, err)
		}
		if env.Kind != "Environment" {
			continue // skip non-Environment documents in multi-doc YAML
		}
		envs = append(envs, env)
	}

	return envs, nil
}
