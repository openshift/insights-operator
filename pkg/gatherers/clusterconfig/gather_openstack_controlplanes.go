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

// GatherOpenstackControlplanes Collects `openstackcontrolplanes.core.openstack.org`
// resources from all namespaces
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openstack/core.openstack.org/openstackcontrolplanes/openstack-galera-network-isolation.json
//
// ### Location in archive
// - `namespaces/{namespace}/core.openstack.org/openstackcontrolplanes/{name}.json`
//
// ### Config ID
// `clusterconfig/openstack_controlplanes`
//
// ### Released version
// - 4.17
//
// ### Changes
// None
func (g *Gatherer) GatherOpenstackControlplanes(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenstackControlplanes(ctx, gatherDynamicClient)
}

func gatherOpenstackControlplanes(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	openstackcontrolplanesList, err := dynamicClient.Resource(oscpGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record

	for i, oscp := range openstackcontrolplanesList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("namespaces/%s/%s/%s/%s",
				oscp.GetNamespace(),
				oscpGroupVersionResource.Group,
				oscpGroupVersionResource.Resource,
				oscp.GetName()),
			Item: record.ResourceMarshaller{Resource: prepareOpenStackControlPlane(&openstackcontrolplanesList.Items[i])},
		})
	}

	return records, nil
}

func prepareOpenStackControlPlane(data *unstructured.Unstructured) *unstructured.Unstructured {
	fieldsToRemove := [][]string{
		{"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
	}
	fieldsToAnonymize := [][]string{
		{"spec", "dns", "template", "options"},
	}
	data.Object = removeFields(data.Object, fieldsToRemove)
	data.Object = anonymizeFields(data.Object, fieldsToAnonymize)
	return data
}
