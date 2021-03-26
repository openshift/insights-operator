package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPDatahubs collects `datahubs.installers.datahub.sap.com` resources from SAP/SDI clusters.
//
// * Location in archive: customresources/installers.datahub.sap.com/datahubs/<namespace>/<name>.json
// * Since versions:
//   * 4.8+
func GatherSAPDatahubs(g *Gatherer, c chan<- gatherResult) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	records, errors := gatherSAPDatahubs(g.ctx, gatherDynamicClient)
	c <- gatherResult{records: records, errors: errors}
}

func gatherSAPDatahubs(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
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
			Name: fmt.Sprintf("customresources/%s/%s/%s/%s",
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
