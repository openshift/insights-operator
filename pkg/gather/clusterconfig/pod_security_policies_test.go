package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var (
	psp1 *policyv1beta1.PodSecurityPolicy = &policyv1beta1.PodSecurityPolicy{
		ObjectMeta: v1.ObjectMeta{Name: "psp-1"},
	}
	psp2 *policyv1beta1.PodSecurityPolicy = &policyv1beta1.PodSecurityPolicy{
		ObjectMeta: v1.ObjectMeta{Name: "psp-2"},
	}
)

func Test_PodSecurityPolicies_Gather(t *testing.T) {
	coreClient := kubefake.NewSimpleClientset()
	ctx := context.Background()
	records, errs := gatherPodSecurityPolicies(ctx, coreClient.PolicyV1beta1())
	assert.Empty(t, errs, "Unexpected errors: %#v", errs)
	assert.Len(t, records, 1)
	s, ok := records[0].Item.(record.JSONMarshaller).Object.([]string)
	assert.True(t, ok, "Unexpected data format. Expecting an array of strings")
	assert.Equal(t, s, []string{}, "Expecting an empty array")

	// create some psps
	_, err := coreClient.PolicyV1beta1().PodSecurityPolicies().Create(ctx, psp1, v1.CreateOptions{})
	assert.NoError(t, err, "Unexpected error when creating test PodSecurityPolicy")
	_, err = coreClient.PolicyV1beta1().PodSecurityPolicies().Create(ctx, psp2, v1.CreateOptions{})
	assert.NoError(t, err, "Unexpected error when creating test PodSecurityPolicy")

	// check that the created PSPs are actually gathered
	records, errs = gatherPodSecurityPolicies(ctx, coreClient.PolicyV1beta1())
	assert.Empty(t, errs, "Unexpected errors: %#v", errs)
	assert.Len(t, records, 1)

	s, ok = records[0].Item.(record.JSONMarshaller).Object.([]string)
	assert.True(t, ok, "Unexpected data format. Expecting an array of strings")
	assert.Equal(t, s, []string{"psp-1", "psp-2"}, "Expecting an empty array")
}
