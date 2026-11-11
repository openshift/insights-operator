package integration_test

import (
	"testing"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

func TestIntegration(t *testing.T) {
	o.RegisterFailHandler(g.Fail)
	g.RunSpecs(t, "Insights Operator Integration Tests")
}
