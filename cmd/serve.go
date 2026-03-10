package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/dcm-io/dcm/pkg/api"
	"github.com/dcm-io/dcm/pkg/scheduler"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
	"github.com/spf13/cobra"
)

var serveAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the DCM API server",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", ":8080", "Address to listen on")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	db, err := store.New(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	seedSampleData(db)

	reg, err := buildRegistryFromDB(db)
	if err != nil {
		return fmt.Errorf("building registry: %w", err)
	}

	server := api.NewServer(db, reg)

	return server.Start(serveAddr)
}

// buildRegistryFromDB loads environments from the database and registers them.
func buildRegistryFromDB(db *store.Store) (*scheduler.Registry, error) {
	factories := buildFactories()
	reg := scheduler.NewRegistry(factories)

	envs, err := db.ListEnvironments()
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}

	for _, rec := range envs {
		if rec.Status != "active" {
			continue
		}
		env := storeEnvToType(rec)
		if err := reg.RegisterEnvironment(env); err != nil {
			return nil, fmt.Errorf("registering environment %q: %w", rec.Name, err)
		}
	}

	log.Printf("Loaded %d environment(s) from database", len(envs))
	return reg, nil
}

// storeEnvToType converts a store.EnvironmentRecord to types.Environment.
func storeEnvToType(rec store.EnvironmentRecord) types.Environment {
	env := types.Environment{
		APIVersion: "dcm.io/v1",
		Kind:       "Environment",
		Metadata: types.Metadata{
			Name:   rec.Name,
			Labels: rec.Labels,
		},
		Spec: types.EnvironmentSpec{
			Provider:     rec.Provider,
			Capabilities: rec.Capabilities,
			Config:       rec.Config,
		},
	}
	if rec.Resources != nil {
		var rp types.ResourcePool
		if json.Unmarshal(rec.Resources, &rp) == nil {
			env.Spec.Resources = &rp
		}
	}
	if rec.Cost != nil {
		var ci types.CostInfo
		if json.Unmarshal(rec.Cost, &ci) == nil {
			env.Spec.Cost = &ci
		}
	}
	return env
}
