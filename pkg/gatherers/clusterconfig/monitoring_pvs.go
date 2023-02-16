package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (g *Gatherer) GatherMonitoring(ctx context.Context) ([]record.Record, []error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	coreClient := kubeClient.CoreV1()
	pvcs := coreClient.PersistentVolumeClaims("openshift-monitoring")
	list, _ := pvcs.List(ctx, v1.ListOptions{})

	var pvs []coreV1.PersistentVolume
	pvint := coreClient.PersistentVolumes()

	var records []record.Record

	for _, pvc := range list.Items {
		PV, _ := pvint.Get(ctx, pvc.Spec.VolumeName, v1.GetOptions{})
		pvs = append(pvs, *PV)

		records = append(records, record.Record{
			Name: fmt.Sprintf(
				"%s/config/pod/%s/%s",
				g.GetName(),
				PV.Namespace,
				PV.Name),
			Item: record.ResourceMarshaller{Resource: PV},
		})
	}

	fmt.Printf("pvs: %v\n", pvs)

	return records, []error{}
}
