package cmd

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/loader"
	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/types"
	"github.com/spf13/cobra"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage and inspect policies",
}

var policyValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate policy files for syntax and CEL expression errors",
	RunE:  runPolicyValidate,
}

var policyEvaluateCmd = &cobra.Command{
	Use:   "evaluate",
	Short: "Evaluate policies against an application and show provider selection",
	RunE:  runPolicyEvaluate,
}

var policyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all loaded policies and their rules",
	RunE:  runPolicyList,
}

func init() {
	rootCmd.AddCommand(policyCmd)
	policyCmd.AddCommand(policyValidateCmd)
	policyCmd.AddCommand(policyEvaluateCmd)
	policyCmd.AddCommand(policyListCmd)
}

func runPolicyValidate(cmd *cobra.Command, args []string) error {
	paths := policyPaths
	if len(paths) == 0 && len(args) > 0 {
		paths = args
	}
	if len(paths) == 0 {
		return fmt.Errorf("specify policy paths with --policy / -p or as arguments")
	}

	policies, err := loader.LoadPolicies(paths)
	if err != nil {
		return fmt.Errorf("loading: %w", err)
	}

	// Attempt to build an evaluator — this validates CEL expressions.
	_, err = policy.NewEvaluator(policies)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Printf("Validated %d policy(ies) with %d total rule(s) — all OK\n",
		len(policies), countRules(policies))
	return nil
}

func runPolicyEvaluate(cmd *cobra.Command, args []string) error {
	paths := policyPaths
	if len(paths) == 0 {
		return fmt.Errorf("specify policy paths with --policy / -p")
	}

	app, err := loader.LoadApplication(appFile)
	if err != nil {
		return err
	}

	policies, err := loader.LoadPolicies(paths)
	if err != nil {
		return fmt.Errorf("loading policies: %w", err)
	}

	evaluator, err := policy.NewEvaluator(policies)
	if err != nil {
		return fmt.Errorf("creating evaluator: %w", err)
	}

	registry := buildRegistry()

	fmt.Printf("\nPolicy evaluation for application: %s\n", app.Metadata.Name)
	fmt.Println("═══════════════════════════════════════════════════════")

	for _, component := range app.Spec.Components {
		result, err := evaluator.Evaluate(&component, app)
		if err != nil {
			return fmt.Errorf("evaluating %s: %w", component.Name, err)
		}

		fmt.Printf("\n  Component: %s (type: %s)\n", component.Name, component.Type)

		if len(result.MatchedRules) == 0 {
			fmt.Println("    Matched rules: (none)")
		} else {
			fmt.Println("    Matched rules:")
			for _, r := range result.MatchedRules {
				fmt.Printf("      - %s\n", r)
			}
		}

		if result.Required != "" {
			fmt.Printf("    Required provider: %s\n", result.Required)
		}
		if len(result.Preferred) > 0 {
			fmt.Printf("    Preferred providers: %v\n", result.Preferred)
		}
		if len(result.Forbidden) > 0 {
			fmt.Printf("    Forbidden providers: %v\n", result.Forbidden)
		}
		if result.Strategy != "" {
			fmt.Printf("    Strategy: %s\n", result.Strategy)
		}
		if len(result.Properties) > 0 {
			fmt.Println("    Injected properties:")
			for k, v := range result.Properties {
				fmt.Printf("      %s: %v\n", k, v)
			}
		}

		// Show which provider would be selected.
		resourceType := types.ResourceType(component.Type)
		selected, err := policy.SelectProvider(result, registry, resourceType)
		if err != nil {
			fmt.Printf("    Selected provider: ERROR — %v\n", err)
		} else {
			fmt.Printf("    Selected provider: %s\n", selected.Name())
		}
	}

	fmt.Println("\n═══════════════════════════════════════════════════════")
	return nil
}

func runPolicyList(cmd *cobra.Command, args []string) error {
	paths := policyPaths
	if len(paths) == 0 && len(args) > 0 {
		paths = args
	}
	if len(paths) == 0 {
		return fmt.Errorf("specify policy paths with --policy / -p or as arguments")
	}

	policies, err := loader.LoadPolicies(paths)
	if err != nil {
		return fmt.Errorf("loading: %w", err)
	}

	fmt.Printf("\n%d policy(ies) loaded\n", len(policies))
	fmt.Println("─────────────────────────────────")

	for _, p := range policies {
		fmt.Printf("\n  Policy: %s\n", p.Metadata.Name)
		for i, rule := range p.Spec.Rules {
			name := rule.Name
			if name == "" {
				name = fmt.Sprintf("rule[%d]", i)
			}
			fmt.Printf("    %s (priority: %d)\n", name, rule.Priority)

			if rule.Match.Type != "" {
				fmt.Printf("      match type: %s\n", rule.Match.Type)
			}
			if len(rule.Match.Labels) > 0 {
				fmt.Printf("      match labels: %v\n", rule.Match.Labels)
			}
			if rule.Match.Expression != "" {
				fmt.Printf("      match expression: %s\n", rule.Match.Expression)
			}
			if rule.Providers.Required != "" {
				fmt.Printf("      required: %s\n", rule.Providers.Required)
			}
			if len(rule.Providers.Preferred) > 0 {
				fmt.Printf("      preferred: %v\n", rule.Providers.Preferred)
			}
			if len(rule.Providers.Forbidden) > 0 {
				fmt.Printf("      forbidden: %v\n", rule.Providers.Forbidden)
			}
			if rule.Providers.Strategy != "" {
				fmt.Printf("      strategy: %s\n", rule.Providers.Strategy)
			}
		}
	}

	fmt.Println("\n─────────────────────────────────")
	return nil
}

func countRules(policies []types.Policy) int {
	n := 0
	for _, p := range policies {
		n += len(p.Spec.Rules)
	}
	return n
}
