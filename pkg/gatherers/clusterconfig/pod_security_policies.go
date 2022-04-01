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
// * Id in config: clusterconfig/psps
// * Since versions:
//   * 4.7.33+
//   * 4.8.12+
//   * 4.9+
func (g *Gatherer) GatherPodSecurityPolicies(ctx context.Context) ([]record.Record, []error) {
	gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPodSecurityPolicies(ctx, gatherPolicyClient)
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
