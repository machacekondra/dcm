package cmd

import (
	"fmt"
	"os"

	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/provider"
	k8sprovider "github.com/dcm-io/dcm/pkg/provider/kubernetes"
	"github.com/dcm-io/dcm/pkg/provider/mock"
	"github.com/dcm-io/dcm/pkg/scheduler"
	"github.com/dcm-io/dcm/pkg/state"
	"github.com/dcm-io/dcm/pkg/store"
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

	evaluator, err := buildEvaluator()
	if err != nil {
		return err
	}

	statePath := resolveStatePath(app.Metadata.Name)
	store := state.NewStore(statePath)

	currentState, err := store.Load(app.Metadata.Name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	schedReg, err := buildSchedulerRegistry()
	if err != nil {
		return err
	}

	sched, err := scheduler.NewScheduler(schedReg, evaluator)
	if err != nil {
		return fmt.Errorf("creating scheduler: %w", err)
	}

	planner := engine.NewPlannerWithScheduler(sched)
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
			fmt.Printf("  + %s (%s via %s%s)\n", step.Component, step.Diff.Type, step.Diff.Provider, envSuffix(step.Environment))
			creates++
		case types.DiffActionUpdate:
			fmt.Printf("  ~ %s (%s via %s%s)\n", step.Component, step.Diff.Type, step.Diff.Provider, envSuffix(step.Environment))
			updates++
		case types.DiffActionDelete:
			fmt.Printf("  - %s (%s via %s%s)\n", step.Component, step.Diff.Type, step.Diff.Provider, envSuffix(step.Environment))
			deletes++
		case types.DiffActionNone:
			fmt.Printf("    %s (no changes)\n", step.Component)
			unchanged++
		}
		if len(step.MatchedRules) > 0 {
			fmt.Printf("      policies: %v\n", step.MatchedRules)
		}
	}

	fmt.Println("─────────────────────────────────")
	fmt.Printf("  %d to create, %d to update, %d to delete, %d unchanged\n\n", creates, updates, deletes, unchanged)
}

func buildFactories() *provider.FactoryRegistry {
	factories := provider.NewFactoryRegistry()

	// Mock provider factory.
	factories.Register("mock", func(config map[string]any) (types.Provider, error) {
		return mock.New(), nil
	})

	// Kubernetes provider factory.
	factories.Register("kubernetes", func(config map[string]any) (types.Provider, error) {
		kubeconfig, _ := config["kubeconfig"].(string)
		context, _ := config["context"].(string)
		namespace, _ := config["namespace"].(string)
		return k8sprovider.New(k8sprovider.Config{
			Kubeconfig: kubeconfig,
			Context:    context,
			Namespace:  namespace,
		})
	})

	return factories
}

func buildSchedulerRegistry() (*scheduler.Registry, error) {
	factories := buildFactories()
	reg := scheduler.NewRegistry(factories)

	if len(envPaths) > 0 {
		// Load from YAML files.
		envs, err := loader.LoadEnvironments(envPaths)
		if err != nil {
			return nil, fmt.Errorf("loading environments: %w", err)
		}
		for _, env := range envs {
			if err := reg.RegisterEnvironment(env); err != nil {
				return nil, fmt.Errorf("registering environment %q: %w", env.Metadata.Name, err)
			}
		}
		fmt.Printf("Loaded %d environment(s) from files\n", len(envs))
	} else {
		// Load from database.
		db, err := store.New(dbPath)
		if err != nil {
			return nil, fmt.Errorf("opening database: %w", err)
		}
		defer db.Close()

		envRecs, err := db.ListEnvironments()
		if err != nil {
			return nil, fmt.Errorf("listing environments: %w", err)
		}

		for _, rec := range envRecs {
			if rec.Status != "active" {
				continue
			}
			env := storeEnvToType(rec)
			if err := reg.RegisterEnvironment(env); err != nil {
				return nil, fmt.Errorf("registering environment %q: %w", rec.Name, err)
			}
		}

		if len(envRecs) > 0 {
			fmt.Printf("Loaded %d environment(s) from database\n", len(envRecs))
		} else {
			// No environments in DB — register mock as fallback.
			reg.RegisterProvider(mock.New())
		}
	}

	return reg, nil
}

func envSuffix(env string) string {
	if env == "" {
		return ""
	}
	return " @ " + env
}

func buildEvaluator() (*policy.Evaluator, error) {
	if len(policyPaths) == 0 {
		return nil, nil
	}

	policies, err := loader.LoadPolicies(policyPaths)
	if err != nil {
		return nil, fmt.Errorf("loading policies: %w", err)
	}

	if len(policies) == 0 {
		return nil, nil
	}

	evaluator, err := policy.NewEvaluator(policies)
	if err != nil {
		return nil, fmt.Errorf("creating policy evaluator: %w", err)
	}

	fmt.Printf("Loaded %d policy(ies)\n", len(policies))
	return evaluator, nil
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
