package cmd

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/state"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the application spec, creating or updating resources",
	RunE:  runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	app, err := loader.LoadApplication(appFile)
	if err != nil {
		return err
	}

	registry := buildRegistry()
	statePath := resolveStatePath(app.Metadata.Name)
	store := state.NewStore(statePath)

	currentState, err := store.Load(app.Metadata.Name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	planner := engine.NewPlanner(registry)
	plan, err := planner.CreatePlan(app, currentState)
	if err != nil {
		return fmt.Errorf("creating plan: %w", err)
	}

	printPlan(plan)

	fmt.Println("Applying changes...")
	executor := engine.NewExecutor(registry)
	if err := executor.Execute(plan, currentState); err != nil {
		return fmt.Errorf("applying: %w", err)
	}

	if err := store.Save(currentState); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Println("\nApply complete.")
	return nil
}
