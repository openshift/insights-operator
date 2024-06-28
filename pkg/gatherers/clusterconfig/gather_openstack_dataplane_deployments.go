// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenstackDataplaneDeployments Collects `openstackdataplanedeployments.dataplane.openstack.org`
// resources from all namespaces
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openstack/dataplane.openstack.org/openstackdataplanedeployments/edpm-deployment.json
//
// ### Location in archive
// - `namespaces/{namespace}/dataplane.openstack.org/openstackdataplanedeployments/{name}.json`
//
// ### Config ID
// `clusterconfig/openstack_dataplanedeployments`
//
// ### Released version
// - 4.17
//
// ### Changes
// None
func (g *Gatherer) GatherOpenstackDataplaneDeployments(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenstackDataplaneDeployments(ctx, gatherDynamicClient)
}

func gatherOpenstackDataplaneDeployments(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	osdpdList, err := dynamicClient.Resource(osdpdGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record

	for i, osdpd := range osdpdList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("namespaces/%s/%s/%s/%s",
				osdpd.GetNamespace(),
				osdpdGroupVersionResource.Group,
				osdpdGroupVersionResource.Resource,
				osdpd.GetName(),
			),
			Item: record.ResourceMarshaller{Resource: prepareOpenStackDataPlaneDeployment(&osdpdList.Items[i])},
		})
	}

	return records, nil
}

func prepareOpenStackDataPlaneDeployment(data *unstructured.Unstructured) *unstructured.Unstructured {
	fieldsToRemove := [][]string{
		{"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
	}
	data.Object = removeFields(data.Object, fieldsToRemove)
	return data
}
