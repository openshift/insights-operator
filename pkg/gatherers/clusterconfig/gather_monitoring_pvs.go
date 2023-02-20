package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	coreV1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// TODO - documentation
func (g *Gatherer) GatherMonitoringPVs(ctx context.Context) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPVsByNamespace(ctx, kubeClient.CoreV1(), "openshift-monitoring")
}

func gatherPVsByNamespace(ctx context.Context, client coreV1.CoreV1Interface, namespace string) ([]record.Record, []error) {
	PVCs := client.PersistentVolumeClaims(namespace)
	pvList, err := PVCs.List(ctx, metaV1.ListOptions{})
	if err != nil {
		return []record.Record{}, []error{err}
	}

	var records []record.Record
	pvInterface := client.PersistentVolumes()
	for i := range pvList.Items {
		pv, _ := pvInterface.Get(ctx, pvList.Items[i].Spec.VolumeName, metaV1.GetOptions{})

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/pod/%s/%s", namespace, pv.Name),
			Item: record.ResourceMarshaller{Resource: pv},
		})
	}

	return records, []error{}
}
