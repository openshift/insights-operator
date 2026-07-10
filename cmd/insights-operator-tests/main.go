package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	"github.com/openshift/insights-operator/test/integration/util"
)

func main() {
	// Create root command
	rootCmd := &cobra.Command{
		Use:   "insights-operator-tests",
		Short: "Insights Operator integration tests",
		Long:  "Integration tests for the Insights Operator following OpenShift Tests Extension framework",
	}

	// Add OTE subcommands (info, list, run-test, run-suite, etc.)
	subcommands := cmd.DefaultExtensionCommands(util.Registry)
	rootCmd.AddCommand(subcommands...)

	// Execute root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

