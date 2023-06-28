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
)

// GatherPodDisruptionBudgets gathers the cluster's PodDisruptionBudgets.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/v11.0.0/kubernetes/typed/policy/v1beta1/poddisruptionbudget.go#L80
// Response see https://docs.okd.io/latest/rest_api/policy_apis/poddisruptionbudget-policy-v1beta1.html
//
// * Location in archive: config/pdbs/
// * See: docs/insights-archive-sample/config/pdbs
// * Id in config: clusterconfig/pdbs
// * Since versions:
//   - 4.4.30+
//   - 4.5.34+
//   - 4.6+
func (g *Gatherer) GatherPodDisruptionBudgets(ctx context.Context) ([]record.Record, []error) {
	gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPodDisruptionBudgets(ctx, gatherPolicyClient)
}

func gatherPodDisruptionBudgets(ctx context.Context, policyClient policyclient.PolicyV1Interface) ([]record.Record, []error) {
	pdbs, err := policyClient.PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	var records []record.Record
	for i := range pdbs.Items {
		item := &pdbs.Items[i]
		if strings.HasPrefix(item.GetNamespace(), "openshift-") {
			recordName := fmt.Sprintf("config/pdbs/%s/%s", item.GetNamespace(), item.GetName())
			records = append(records, record.Record{
				Name: recordName,
				Item: record.ResourceMarshaller{Resource: item},
			})
			if len(records) == gatherPodDisruptionBudgetLimit {
				break
			}
		}
	}
	return records, nil
}
