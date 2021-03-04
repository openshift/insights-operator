package clusterconfig

import (
	"context"
	"fmt"

	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1beta1"

	"github.com/openshift/insights-operator/pkg/record"
)

const (
	gatherPodDisruptionBudgetLimit = 5000
)

var (
	policyV1Beta1Serializer = kubescheme.Codecs.LegacyCodec(policyv1beta1.SchemeGroupVersion)
)

type PodDisruptionBudgetsAnonymizer struct {
	*policyv1beta1.PodDisruptionBudget
}

// GatherPodDisruptionBudgets gathers the cluster's PodDisruptionBudgets.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/v11.0.0/kubernetes/typed/policy/v1beta1/poddisruptionbudget.go#L80
// Response see https://docs.okd.io/latest/rest_api/policy_apis/poddisruptionbudget-policy-v1beta1.html
//
// Location in archive: config/pdbs/
// See: docs/insights-archive-sample/config/pdbs
func GatherPodDisruptionBudgets(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		gatherPolicyClient, err := policyclient.NewForConfig(g.gatherKubeConfig)
		if err != nil {
			return nil, []error{err}
		}
		return gatherPodDisruptionBudgets(g.ctx, gatherPolicyClient)
	}
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
			Item: PodDisruptionBudgetsAnonymizer{&pdb},
		})
	}
	return records, nil
}

// Marshal implements serialization of a PodDisruptionBudget with anonymization
func (a PodDisruptionBudgetsAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(policyV1Beta1Serializer, a.PodDisruptionBudget)
}

// GetExtension returns extension for anonymized PodDisruptionBudget objects
func (a PodDisruptionBudgetsAnonymizer) GetExtension() string {
	return "json"
}
