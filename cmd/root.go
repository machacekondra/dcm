package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	appFile     string
	stateFile   string
	policyPaths []string
	envPaths    []string
)

var rootCmd = &cobra.Command{
	Use:   "dcm",
	Short: "DCM — Declarative Cloud Manager",
	Long:  "A platform engineering tool for building and deploying complex applications with dependency management, policy-driven provider selection, and GitOps support.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&appFile, "file", "f", "app.yaml", "Path to the application spec file")
	rootCmd.PersistentFlags().StringVar(&stateFile, "state", "", "Path to the state file (defaults to <app-name>.dcm.state)")
	rootCmd.PersistentFlags().StringSliceVarP(&policyPaths, "policy", "p", nil, "Paths to policy files or directories (can be repeated)")
	rootCmd.PersistentFlags().StringSliceVarP(&envPaths, "environment", "e", nil, "Paths to environment files or directories (can be repeated)")
}
