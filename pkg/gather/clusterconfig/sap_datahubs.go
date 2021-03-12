package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPDatahubs collects `datahubs.installers.datahub.sap.com` resources from SAP/SDI clusters.
//
// Location in archive: config/installers.datahub.sap.com/datahubs/<namespace>/<name>.json
func GatherSAPDatahubs(g *Gatherer, c chan<- gatherResult) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	records, errors := gatherSAPDatahubs(g.ctx, gatherDynamicClient, gatherKubeClient.CoreV1())
	c <- gatherResult{records: records, errors: errors}
}

func gatherSAPDatahubs(ctx context.Context, dynamicClient dynamic.Interface, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	datahubsList, err := dynamicClient.Resource(datahubGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{}

	for i, datahub := range datahubsList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/%s/%s/%s/%s",
				datahubGroupVersionResource.Group,
				datahubGroupVersionResource.Resource,
				datahub.GetNamespace(),
				datahub.GetName(),
			),
			Item: record.JSONMarshaller{Object: &datahubsList.Items[i]},
		})
	}

	return records, nil
}
