package util

import (
	"github.com/openshift-eng/openshift-tests-extension/pkg/extension"
)

// Registry is the OTE extension registry for insights-operator tests
var Registry = extension.NewRegistry()

// Ext is the insights-operator test extension
var Ext *extension.Extension

func init() {
	// Create the insights-operator test extension
	// Args: product, kind, name
	Ext = extension.NewExtension("insights-operator", "openshift", "insights-operator")

	// Add test suites
	Ext.Suites = []extension.Suite{
		{
			Name:        "insights-operator/all",
			Description: "All insights-operator integration tests",
		},
		{
			Name:        "insights-operator/gatherers",
			Description: "Gatherer content validation tests",
		},
		{
			Name:        "insights-operator/controllers",
			Description: "Controller behavior tests",
		},
	}

	// Register the extension
	Registry.Register(Ext)
}
