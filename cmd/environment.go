package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
	"github.com/spf13/cobra"
)

var environmentCmd = &cobra.Command{
	Use:     "environment",
	Aliases: []string{"env"},
	Short:   "Manage and inspect environments",
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments from the database",
	RunE:  runEnvList,
}

var envDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an environment from the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvDelete,
}

var envValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate environment YAML files without storing",
	RunE:  runEnvValidate,
}

var envCreateCmd = &cobra.Command{
	Use:   "create -f <environment.yaml>",
	Short: "Create environments from a YAML file into the database",
	Long: `Create environments from a YAML file, validating and storing them in the database.

Examples:
  dcm environment create -f environments/prod.yaml
  dcm environment create -f examples/environments/clusters.yaml`,
	RunE: runEnvCreate,
}

var envCreateFile string

func init() {
	rootCmd.AddCommand(environmentCmd)
	environmentCmd.AddCommand(envListCmd)
	environmentCmd.AddCommand(envCreateCmd)
	environmentCmd.AddCommand(envDeleteCmd)
	environmentCmd.AddCommand(envValidateCmd)

	envCreateCmd.Flags().StringVarP(&envCreateFile, "file", "f", "", "Path to environment YAML file (required)")
	_ = envCreateCmd.MarkFlagRequired("file")
}

func openEnvDB() (*store.Store, error) {
	db, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", dbPath, err)
	}
	return db, nil
}

func runEnvCreate(cmd *cobra.Command, args []string) error {
	envs, err := loader.LoadEnvironments([]string{envCreateFile})
	if err != nil {
		return fmt.Errorf("loading %s: %w", envCreateFile, err)
	}

	factories := buildFactories()

	for _, e := range envs {
		if e.Metadata.Name == "" {
			return fmt.Errorf("environment missing name")
		}
		if e.Spec.Provider == "" {
			return fmt.Errorf("environment %q missing provider", e.Metadata.Name)
		}
		if !factories.Has(e.Spec.Provider) {
			return fmt.Errorf("environment %q: unknown provider type %q (available: %v)",
				e.Metadata.Name, e.Spec.Provider, factories.Types())
		}
	}

	db, err := openEnvDB()
	if err != nil {
		return err
	}
	defer db.Close()

	for _, e := range envs {
		rec := envToRecord(e)
		if err := db.CreateEnvironment(rec); err != nil {
			return fmt.Errorf("creating environment %q: %w", e.Metadata.Name, err)
		}
		fmt.Printf("Created environment %q (provider: %s)\n", e.Metadata.Name, e.Spec.Provider)
	}

	fmt.Printf("\n%d environment(s) created from %s\n", len(envs), envCreateFile)
	return nil
}

func runEnvList(cmd *cobra.Command, args []string) error {
	db, err := openEnvDB()
	if err != nil {
		return err
	}
	defer db.Close()

	envs, err := db.ListEnvironments()
	if err != nil {
		return fmt.Errorf("listing environments: %w", err)
	}

	if len(envs) == 0 {
		fmt.Println("No environments found.")
		return nil
	}

	fmt.Printf("\n%d environment(s)\n", len(envs))
	fmt.Println("─────────────────────────────────")

	for _, e := range envs {
		fmt.Printf("\n  Environment: %s\n", e.Name)
		fmt.Printf("    Provider: %s\n", e.Provider)
		fmt.Printf("    Status:   %s\n", e.Status)

		if len(e.Labels) > 0 {
			fmt.Printf("    Labels:   %v\n", e.Labels)
		}

		if e.Resources != nil {
			var rp types.ResourcePool
			if json.Unmarshal(e.Resources, &rp) == nil {
				fmt.Printf("    Resources: cpu=%d, memory=%d, pods=%d\n", rp.CPU, rp.Memory, rp.Pods)
			}
		}

		if e.Cost != nil {
			var ci types.CostInfo
			if json.Unmarshal(e.Cost, &ci) == nil {
				fmt.Printf("    Cost: tier=%s, rate=%.4f\n", ci.Tier, ci.HourlyRate)
			}
		}

		fmt.Printf("    Created:  %s\n", e.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("\n─────────────────────────────────")
	return nil
}

func runEnvDelete(cmd *cobra.Command, args []string) error {
	db, err := openEnvDB()
	if err != nil {
		return err
	}
	defer db.Close()

	name := args[0]
	if err := db.DeleteEnvironment(name); err != nil {
		return fmt.Errorf("deleting environment %q: %w", name, err)
	}

	fmt.Printf("Deleted environment %q\n", name)
	return nil
}

func envToRecord(e types.Environment) *store.EnvironmentRecord {
	var resources, cost json.RawMessage
	if e.Spec.Resources != nil {
		r, _ := json.Marshal(e.Spec.Resources)
		resources = r
	}
	if e.Spec.Cost != nil {
		c, _ := json.Marshal(e.Spec.Cost)
		cost = c
	}
	return &store.EnvironmentRecord{
		Name:      e.Metadata.Name,
		Provider:  e.Spec.Provider,
		Labels:    e.Metadata.Labels,
		Config:    e.Spec.Config,
		Resources: resources,
		Cost:      cost,
		Status:    "active",
	}
}

func runEnvValidate(cmd *cobra.Command, args []string) error {
	paths := envPaths
	if len(paths) == 0 && len(args) > 0 {
		paths = args
	}
	if len(paths) == 0 {
		return fmt.Errorf("specify environment paths with --environment / -e or as arguments")
	}

	envs, err := loader.LoadEnvironments(paths)
	if err != nil {
		return fmt.Errorf("loading: %w", err)
	}

	// Validate each environment has required fields.
	for _, e := range envs {
		if e.Metadata.Name == "" {
			return fmt.Errorf("environment missing name")
		}
		if e.Spec.Provider == "" {
			return fmt.Errorf("environment %q missing provider", e.Metadata.Name)
		}
	}

	// Try to create provider instances.
	factories := buildFactories()
	for _, e := range envs {
		if !factories.Has(e.Spec.Provider) {
			return fmt.Errorf("environment %q: unknown provider type %q", e.Metadata.Name, e.Spec.Provider)
		}
	}

	fmt.Printf("Validated %d environment(s) — all OK\n", len(envs))
	return nil
}
