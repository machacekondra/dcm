package cmd

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/state"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current status of deployed resources",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	app, err := loader.LoadApplication(appFile)
	if err != nil {
		return err
	}

	statePath := resolveStatePath(app.Metadata.Name)
	store := state.NewStore(statePath)

	currentState, err := store.Load(app.Metadata.Name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if len(currentState.Resources) == 0 {
		fmt.Println("No resources deployed.")
		return nil
	}

	fmt.Printf("\nStatus for application: %s\n", app.Metadata.Name)
	fmt.Println("─────────────────────────────────")

	for name, resource := range currentState.Resources {
		fmt.Printf("  %-20s %-15s %-10s %s\n", name, resource.Type, resource.Provider, resource.Status)
		if len(resource.Outputs) > 0 {
			for k, v := range resource.Outputs {
				fmt.Printf("    %s: %v\n", k, v)
			}
		}
	}

	fmt.Println("─────────────────────────────────")
	fmt.Printf("  %d resources total\n\n", len(currentState.Resources))
	return nil
}
