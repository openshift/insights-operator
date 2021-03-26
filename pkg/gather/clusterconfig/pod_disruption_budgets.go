package clusterconfig

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1beta1"

	"github.com/openshift/insights-operator/pkg/record"
)

const (
	gatherPodDisruptionBudgetLimit = 5000
)

// GatherPodDisruptionBudgets gathers the cluster's PodDisruptionBudgets.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/v11.0.0/kubernetes/typed/policy/v1beta1/poddisruptionbudget.go#L80
// Response see https://docs.okd.io/latest/rest_api/policy_apis/poddisruptionbudget-policy-v1beta1.html
//
// * Location in archive: config/pdbs/
// * See: docs/insights-archive-sample/config/pdbs
// * Id in config: pdbs
// * Since versions:
//   * 4.4.30+
//   * 4.5.34+
//   * 4.6+
func GatherPodDisruptionBudgets(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherPodDisruptionBudgets(g.ctx, gatherPolicyClient)
	c <- gatherResult{records, errors}
}

func gatherPodDisruptionBudgets(ctx context.Context, policyClient policyclient.PolicyV1beta1Interface) ([]record.Record, []error) {
	pdbs, err := policyClient.PodDisruptionBudgets("").List(ctx, metav1.ListOptions{Limit: gatherPodDisruptionBudgetLimit})
	if err != nil {
		return nil, []error{err}
	}
	records := []record.Record{}
	for _, pdb := range pdbs.Items {
		recordName := fmt.Sprintf("config/pdbs/%s", pdb.GetName())
		if pdb.GetNamespace() != "" {
			recordName = fmt.Sprintf("config/pdbs/%s/%s", pdb.GetNamespace(), pdb.GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.JSONMarshaller{Object: pdb},
		})
	}
	return records, nil
}
