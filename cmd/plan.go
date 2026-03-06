package cmd

import (
	"fmt"
	"os"

	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/provider"
	k8sprovider "github.com/dcm-io/dcm/pkg/provider/kubernetes"
	"github.com/dcm-io/dcm/pkg/provider/mock"
	"github.com/dcm-io/dcm/pkg/state"
	"github.com/dcm-io/dcm/pkg/types"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show the execution plan for an application",
	RunE:  runPlan,
}

func init() {
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
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
	return nil
}

func printPlan(plan *engine.Plan) {
	fmt.Printf("\nPlan for application: %s\n", plan.AppName)
	fmt.Println("─────────────────────────────────")

	creates, updates, deletes, unchanged := 0, 0, 0, 0
	for _, step := range plan.Steps {
		switch step.Diff.Action {
		case types.DiffActionCreate:
			fmt.Printf("  + %s (%s via %s)\n", step.Component, step.Diff.Type, step.Diff.Provider)
			creates++
		case types.DiffActionUpdate:
			fmt.Printf("  ~ %s (%s via %s)\n", step.Component, step.Diff.Type, step.Diff.Provider)
			updates++
		case types.DiffActionDelete:
			fmt.Printf("  - %s (%s via %s)\n", step.Component, step.Diff.Type, step.Diff.Provider)
			deletes++
		case types.DiffActionNone:
			fmt.Printf("    %s (no changes)\n", step.Component)
			unchanged++
		}
	}

	fmt.Println("─────────────────────────────────")
	fmt.Printf("  %d to create, %d to update, %d to delete, %d unchanged\n\n", creates, updates, deletes, unchanged)
}

func buildRegistry() *provider.Registry {
	registry := provider.NewRegistry()

	// Always register the mock provider for testing.
	registry.Register(mock.New())

	// Register Kubernetes provider if a kubeconfig is available.
	k8s, err := k8sprovider.New(k8sprovider.Config{})
	if err == nil {
		registry.Register(k8s)
	}

	return registry
}

func resolveStatePath(appName string) string {
	if stateFile != "" {
		return stateFile
	}
	return state.DefaultPath(appName)
}

func exitOnError(msg string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
	os.Exit(1)
}
