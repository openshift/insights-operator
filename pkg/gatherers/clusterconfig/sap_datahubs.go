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
//   * 4.7.5+
//   * 4.8+
func (g *Gatherer) GatherSAPDatahubs(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSAPDatahubs(ctx, gatherDynamicClient)
}

func gatherSAPDatahubs(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	datahubsList, err := dynamicClient.Resource(datahubGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record

	for i, datahub := range datahubsList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("customresources/%s/%s/%s/%s",
				datahubGroupVersionResource.Group,
				datahubGroupVersionResource.Resource,
				datahub.GetNamespace(),
				datahub.GetName(),
			),
			Item: record.ResourceMarshaller{Resource: &datahubsList.Items[i]},
		})
	}

	return records, nil
}
