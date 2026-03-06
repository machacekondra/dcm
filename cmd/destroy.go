package cmd

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/state"
	"github.com/dcm-io/dcm/pkg/types"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy all resources managed by the application",
	RunE:  runDestroy,
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}

func runDestroy(cmd *cobra.Command, args []string) error {
	app, err := loader.LoadApplication(appFile)
	if err != nil {
		return err
	}

	schedReg, err := buildSchedulerRegistry()
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
		fmt.Println("No resources to destroy.")
		return nil
	}

	fmt.Printf("\nDestroying application: %s\n", app.Metadata.Name)
	fmt.Println("─────────────────────────────────")

	for name, resource := range currentState.Resources {
		provider, ok := schedReg.Get(resource.Provider)
		if !ok {
			return fmt.Errorf("provider %q not found for resource %q", resource.Provider, name)
		}

		fmt.Printf("  - %s (%s via %s)\n", name, resource.Type, resource.Provider)
		if err := provider.Destroy(resource); err != nil {
			return fmt.Errorf("destroying %s: %w", name, err)
		}
	}

	// Clear state.
	currentState.Resources = make(map[string]*types.Resource)
	if err := store.Save(currentState); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Println("─────────────────────────────────")
	fmt.Println("Destroy complete.")
	return nil
}
