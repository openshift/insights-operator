package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherVirtualMachineInstances Collects `VirtualMachineInstance` resources from cluster if available.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/virtualmachineinstances/default/fedora-r2nf0eocvxbkmqjy.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | ---------------------------------------------------------- |
// | >= 4.14   | config/virtualmachineinstances/{namespace}/{name}.json		|
//
// ### Config ID
// `clusterconfig/virtual_machine_instances`
//
// ### Released version
// - 4.14
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherVirtualMachineInstances(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherVirtualMachineInstances(ctx, gatherDynamicClient)
}

func gatherVirtualMachineInstances(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	virtualizationList, err := dynamicClient.Resource(virtualMachineInstancesResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i := range virtualizationList.Items {
		item := &virtualizationList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/virtualmachineinstances/%s/%s", item.GetNamespace(), item.GetName()),
			Item: record.ResourceMarshaller{Resource: anonymizeVirtualMachineInstances(item)},
		})
	}

	return records, nil
}

func anonymizeVirtualMachineInstances(data *unstructured.Unstructured) *unstructured.Unstructured {
	const errMsg = "error during anonymizing virtualmachineinstances:"
	volumes, found, err := unstructured.NestedSlice(data.Object, "spec", "volumes")
	if !found || err != nil {
		klog.Infof("%s unable to find volumes %v %v", errMsg, found, err)
		return data
	}

	for i := range volumes {
		volume, ok := volumes[i].(map[string]interface{})
		if !ok {
			klog.Infof("%s volumes is not a map", errMsg)
			continue
		}

		_, ok = volume["cloudInitNoCloud"]
		if !ok {
			klog.Infof("%s cloudInitNoCloud not found", errMsg)
			continue
		}

		volume["cloudInitNoCloud"] = ""
	}

	err = unstructured.SetNestedSlice(data.Object, volumes, "spec", "volumes")
	if err != nil {
		klog.Infof("%s unable to set anonymized volumes: %v", errMsg, err.Error())
	}

	return data
}
