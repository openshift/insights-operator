package integration_test

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	// Import test suites to register Ginkgo tests
	_ "github.com/openshift/insights-operator/test/integration/gatherers"
)

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Insights Operator Integration Tests")
}
