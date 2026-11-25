package test

import (
	"context"
	"testing"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	testutil "github.com/openshift/insights-operator/tests-extension/test/util"
)

var _ = g.BeforeSuite(func(ctx context.Context) {
	o.Expect(testutil.Setup(ctx)).To(o.Succeed())
})

func TestIntegration(t *testing.T) {
	o.RegisterFailHandler(g.Fail)
	g.RunSpecs(t, "Insights Operator Integration Tests")
}
