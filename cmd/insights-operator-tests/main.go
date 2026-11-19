package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	"github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"

	_ "github.com/openshift/insights-operator/test/integration"
)

func main() {
	registry := extension.NewRegistry()
	ext := extension.NewExtension("openshift", "payload", "insights-operator")

	ext.AddSuite(extension.Suite{
		Name:        "insights-operator/all",
		Description: "All insights-operator integration tests",
	})

	specs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		panic(fmt.Sprintf("couldn't build extension test specs from ginkgo: %+v", err.Error()))
	}

	ext.AddSpecs(specs)
	registry.Register(ext)

	// Create root command
	rootCmd := &cobra.Command{
		Use:   "insights-operator-tests",
		Short: "Insights Operator integration tests",
		Long:  "Integration tests for the Insights Operator following OpenShift Tests Extension framework",
	}

	// Add OTE subcommands (info, list, run-test, run-suite, etc.)
	subcommands := cmd.DefaultExtensionCommands(registry)
	rootCmd.AddCommand(subcommands...)

	// Execute root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
