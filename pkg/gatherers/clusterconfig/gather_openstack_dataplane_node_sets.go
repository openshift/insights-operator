package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherOpenstackDataplaneNodesets Collects `openstackdataplanenodesets.core.openstack.org`
// resources from all namespaces
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/customresources/dataplane.openstack.org/openstackdataplanenodesets/openstack/openstack-edpm.json
//
// ### Location in archive
// - `customresources/dataplane.openstack.org/openstackdataplanes/{namespace}/{name}.json`
//
// ### Config ID
// `clusterconfig/openstack_dataplane_nodesets`
//
// ### Released version
// - 4.15
//
// ### Changes
// None
func (g *Gatherer) GatherOpenstackDataplaneNodeSets(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenstackDataplaneNodeSets(ctx, gatherDynamicClient)
}

func gatherOpenstackDataplaneNodeSets(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	osdpnsList, err := dynamicClient.Resource(osdpnsGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record

	for i, osdpns := range osdpnsList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("customresources/%s/%s/%s/%s",
				osdpnsGroupVersionResource.Group,
				osdpnsGroupVersionResource.Resource,
				osdpns.GetNamespace(),
				osdpns.GetName(),
			),
			Item: record.ResourceMarshaller{Resource: prepareOpenStackDataPlaneNodeSet(&osdpnsList.Items[i])},
		})
	}

	return records, nil
}

func prepareOpenStackDataPlaneNodeSet(data *unstructured.Unstructured) *unstructured.Unstructured {
	fieldsToRemove := [][]string{
		{"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
	}
	fieldsToAnonymize := [][]string{}
	customFieldsToAnonymize := []string{
		"ansibleUser", "edpm_sshd_allowed_ranges", "dnsClusterAddresses",
	}
	data.Object = removeFields(data.Object, fieldsToRemove)
	data.Object = anonymizeIpAddresses(data.Object)
	data.Object = anonymizeFields(data.Object, fieldsToAnonymize)
	data.Object = anonymizeCustomPathFields(data.Object, customFieldsToAnonymize)
	data.Object = anonymizeStatusHostNames(data.Object)
	return data
}

func anonymizeStatusHostNames(data map[string]interface{}) map[string]interface{} {
	allHosts, found, err := unstructured.NestedMap(data, "status", "allHostnames")
	if !found {
		klog.Infof("no 'allHostnames' field found in the OpenStackDataPlaneNodeSet CR")
		return data
	}
	if err != nil {
		klog.Infof("error during anonymization of the OpenStackDataPlaneNodeSet CR")
		return data
	}
	for host, networks := range allHosts {
		// allHostnames has got structure like ;
		//   edpm-node-name:
		//	   net_name: hostname
		// It is not expected to have different format of that field so there's no need
		// to bother with different possibilities
		networksList := networks.(map[string]interface{})
		for network, hostname := range networksList {
			hostnameStr, _ := hostname.(string)
			unstructured.SetNestedField(data, anonymize.String(hostnameStr), "status", "allHostnames", host, network)
		}
	}
	return data
}
