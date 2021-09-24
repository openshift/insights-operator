package clusterconfig

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
)

// GatherPodSecurityPolicies gathers the names of installed PodSecurityPolicies
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/v12.0.0/kubernetes/typed/policy/v1beta1/podsecuritypolicy.go#L76
//
// * Location in archive: config/psp_names.json
// * See: docs/insights-archive-sample/config/psp_names.json
// * Id in config: psps
// * Since versions:
//   * 4.10+
func GatherPodSecurityPolicies(g *Gatherer, c chan<- gatherResult) {
	gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
	}
	records, errs := gatherPodSecurityPolicies(g.ctx, gatherPolicyClient)
	c <- gatherResult{records: records, errors: errs}
}

func gatherPodSecurityPolicies(ctx context.Context, policyClient policyclient.PolicyV1beta1Interface) ([]record.Record, []error) {
	psps, err := policyClient.PodSecurityPolicies().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	pspNames := make([]string, 0, len(psps.Items))
	for i := range psps.Items {
		psp := psps.Items[i]
		pspNames = append(pspNames, psp.Name)
	}
	return []record.Record{{
		Name: "config/psp_names",
		Item: record.JSONMarshaller{Object: pspNames},
	}}, nil
}
