// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenstackVersions Collects `openstackversion.core.openstack.org`
// resources from all namespaces
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openstack/core.openstack.org/openstackversion/openstack-galera-network-isolation.json
//
// ### Location in archive
// - `namespaces/{namespace}/core.openstack.org/openstackversion/{name}.json`
//
// ### Config ID
// `clusterconfig/openstack_version`
//
// ### Released version
// - 4.17
//
// ### Changes
// None
func (g *Gatherer) GatherOpenstackVersions(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return GatherOpenstackVersions(ctx, gatherDynamicClient)
}

func GatherOpenstackVersions(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	openstackversionsList, err := dynamicClient.Resource(osvGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i, osv := range openstackversionsList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("namespaces/%s/%s/%s/%s",
				osv.GetNamespace(),
				osvGroupVersionResource.Group,
				osvGroupVersionResource.Resource,
				osv.GetName()),
			Item: record.ResourceMarshaller{Resource: &openstackversionsList.Items[i]},
		})
	}

	return records, nil
}
