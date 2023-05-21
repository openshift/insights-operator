package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

const (
	gatherPodDisruptionBudgetLimit = 100
	serverRequestLimit             = 5000
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
// - `config/pdbs/{namespace}/{name}.json`
//
// ### Config ID
// `clusterconfig/pdbs`
//
// ### Released version
// - 4.6.0
//
// ### Backported versions
// - 4.4.30+
// - 4.5.15+
//
// ### Changes
// - The gatherer was changed to gather pdbs only from namespaces with "openshift" prefix
// and the limit of gathered records to 100 since 4.14.
func (g *Gatherer) GatherPodDisruptionBudgets(ctx context.Context) ([]record.Record, []error) {
	gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPodDisruptionBudgets(ctx, gatherPolicyClient)
}

func gatherPodDisruptionBudgets(ctx context.Context, policyClient policyclient.PolicyV1Interface) ([]record.Record, []error) {
	pdbs, err := policyClient.PodDisruptionBudgets("").List(ctx, metav1.ListOptions{Limit: serverRequestLimit})
	if err != nil {
		return nil, []error{err}
	}
	var records []record.Record
	limit := 0
	for i := range pdbs.Items {
		if strings.HasPrefix(pdbs.Items[i].GetNamespace(), "openshift") {
			limit++
			if limit == gatherPodDisruptionBudgetLimit {
				break
			}
			recordName := fmt.Sprintf("config/pdbs/%s/%s", pdbs.Items[i].GetNamespace(), pdbs.Items[i].GetName())
			records = append(records, record.Record{
				Name: recordName,
				Item: record.ResourceMarshaller{Resource: &pdbs.Items[i]},
			})
		}
	}
	return records, nil
}
