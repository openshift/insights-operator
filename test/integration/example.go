package integration_test

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[smoke] Test Framework Verification", func() {
	g.It("should always pass", func() {
		o.Expect(true).To(o.BeTrue())
	})
})
