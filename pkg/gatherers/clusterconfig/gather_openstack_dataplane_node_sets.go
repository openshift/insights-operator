// nolint: dupl
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

// GatherOpenstackDataplaneNodesets Collects `openstackdataplanenodesets.dataplane.openstack.org`
// resources from all namespaces
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/openstack/dataplane.openstack.org/openstackdataplanenodesets/openstack-edpm.json
//
// ### Location in archive
// - `namespaces/{namespace}/dataplane.openstack.org/openstackdataplanes/{name}.json`
//
// ### Config ID
// `clusterconfig/openstack_dataplanenodesets`
//
// ### Released version
// - 4.17
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
			Name: fmt.Sprintf("namespaces/%s/%s/%s/%s",
				osdpns.GetNamespace(),
				osdpnsGroupVersionResource.Group,
				osdpnsGroupVersionResource.Resource,
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
		"ansibleHost", "ansibleUser", "edpm_sshd_allowed_ranges", "dnsClusterAddresses",
	}
	data.Object = removeFields(data.Object, fieldsToRemove)
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
		networksList, ok := networks.(map[string]interface{})
		if !ok {
			klog.Warningf("error while converting host %s allHosts networks to map[string]interface{}", host)
			continue
		}
		for network, hostname := range networksList {
			hostnameStr, ok := hostname.(string)
			if !ok {
				klog.Warningf("error while converting hostname '%s' to string", hostname)
				continue
			}
			err := unstructured.SetNestedField(data, anonymize.String(hostnameStr), "status", "allHostnames", host, network)
			if err != nil {
				klog.Infof("error during annonymization of the hostname '%s'; error: '%s'", hostnameStr, err)
				continue
			}
		}
	}
	return data
}
