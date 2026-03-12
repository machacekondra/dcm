package cmd

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
	"gopkg.in/yaml.v3"
)

// seedFromDataDir loads YAML files from the given directory and seeds
// applications, environments, and policies that don't already exist.
func seedFromDataDir(db *store.Store, dataDir string) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("seed: data directory %s not found, skipping", dataDir)
			return
		}
		log.Printf("seed: failed to read data directory %s: %v", dataDir, err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry.Name()) {
			continue
		}
		path := filepath.Join(dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("seed: failed to read %s: %v", path, err)
			continue
		}
		if err := seedFile(db, path, data); err != nil {
			log.Printf("seed: %s: %v", path, err)
		}
	}
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

// header is used to peek at the Kind field before full unmarshalling.
type header struct {
	Kind string `yaml:"kind"`
}

func seedFile(db *store.Store, path string, data []byte) error {
	var h header
	if err := yaml.Unmarshal(data, &h); err != nil {
		return err
	}

	switch h.Kind {
	case "Application":
		return seedApplication(db, path, data)
	case "Environment":
		return seedEnvironment(db, path, data)
	case "Policy":
		return seedPolicy(db, path, data)
	default:
		log.Printf("seed: %s: unknown kind %q, skipping", path, h.Kind)
		return nil
	}
}

func seedApplication(db *store.Store, path string, data []byte) error {
	var app types.Application
	if err := yaml.Unmarshal(data, &app); err != nil {
		return err
	}

	name := app.Metadata.Name
	if _, err := db.GetApplication(name); err == nil {
		return nil // already exists
	}

	componentsJSON, err := json.Marshal(app.Spec.Components)
	if err != nil {
		return err
	}

	rec := &store.ApplicationRecord{
		Name:       name,
		Labels:     app.Metadata.Labels,
		Components: componentsJSON,
	}
	if err := db.CreateApplication(rec); err != nil {
		return err
	}
	log.Printf("seed: created application %q from %s", name, filepath.Base(path))
	return nil
}

func seedEnvironment(db *store.Store, path string, data []byte) error {
	var env types.Environment
	if err := yaml.Unmarshal(data, &env); err != nil {
		return err
	}

	name := env.Metadata.Name

	rec := &store.EnvironmentRecord{
		Name:         name,
		Provider:     env.Spec.Provider,
		Labels:       env.Metadata.Labels,
		Capabilities: env.Spec.Capabilities,
		Config:       env.Spec.Config,
		Status:       "active",
	}

	if env.Spec.Resources != nil {
		j, _ := json.Marshal(env.Spec.Resources)
		rec.Resources = j
	}
	if env.Spec.Cost != nil {
		j, _ := json.Marshal(env.Spec.Cost)
		rec.Cost = j
	}
	if env.Spec.HealthCheck != nil {
		j, _ := json.Marshal(env.Spec.HealthCheck)
		rec.HealthCheck = j
	}

	if _, err := db.GetEnvironment(name); err == nil {
		// Already exists — update it from the YAML.
		if err := db.UpdateEnvironment(rec); err != nil {
			return err
		}
		log.Printf("seed: updated environment %q from %s", name, filepath.Base(path))
		return nil
	}

	if err := db.CreateEnvironment(rec); err != nil {
		return err
	}
	log.Printf("seed: created environment %q from %s", name, filepath.Base(path))
	return nil
}

func seedPolicy(db *store.Store, path string, data []byte) error {
	var pol types.Policy
	if err := yaml.Unmarshal(data, &pol); err != nil {
		return err
	}

	name := pol.Metadata.Name
	if _, err := db.GetPolicy(name); err == nil {
		return nil // already exists
	}

	rulesJSON, err := json.Marshal(pol.Spec.Rules)
	if err != nil {
		return err
	}

	rec := &store.PolicyRecord{
		Name:  name,
		Rules: rulesJSON,
	}
	if err := db.CreatePolicy(rec); err != nil {
		return err
	}
	log.Printf("seed: created policy %q from %s", name, filepath.Base(path))
	return nil
}
