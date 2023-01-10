package clusterconfig

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

const (
	gatherPodDisruptionBudgetLimit = 5000
)

// GatherPodDisruptionBudgets Collects the cluster's `PodDisruptionBudgets`.
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/v11.0.0/kubernetes/typed/policy/v1beta1/poddisruptionbudget.go#L80
// - https://docs.okd.io/latest/rest_api/policy_apis/poddisruptionbudget-policy-v1beta1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/pdbs/openshift-machine-config-operator/etcd-quorum-guard.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// |  >= 4.6.0 | config/pdbs/{namespace}/{name}.json	                    |
//
// ### Config ID
// `clusterconfig/pdbs`
//
// ### Released version
// - 4.6.0
//
// ### Backported versions
// - 4.5.15+
// - 4.4.30+
//
// ### Notes
// None
func (g *Gatherer) GatherPodDisruptionBudgets(ctx context.Context) ([]record.Record, []error) {
	gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPodDisruptionBudgets(ctx, gatherPolicyClient)
}

func gatherPodDisruptionBudgets(ctx context.Context, policyClient policyclient.PolicyV1Interface) ([]record.Record, []error) {
	pdbs, err := policyClient.PodDisruptionBudgets("").List(ctx, metav1.ListOptions{Limit: gatherPodDisruptionBudgetLimit})
	if err != nil {
		return nil, []error{err}
	}
	var records []record.Record
	for i := range pdbs.Items {
		recordName := fmt.Sprintf("config/pdbs/%s", pdbs.Items[i].GetName())
		if pdbs.Items[i].GetNamespace() != "" {
			recordName = fmt.Sprintf("config/pdbs/%s/%s", pdbs.Items[i].GetNamespace(), pdbs.Items[i].GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: &pdbs.Items[i]},
		})
	}
	return records, nil
}
