package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/api/errors"

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
// - docs/insights-archive-sample/config/virtualmachineinstances/openshift-cnv/fedora-r2nf0eocvxbkmqjy.json
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
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var errs []error
	// Limit the number of gathered virtualmachineinstances.kubevirt.io
	var limit = 5
	records := make([]record.Record, 0, limit)
	for i := range virtualizationList.Items {
		item := &virtualizationList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/virtualmachineinstances/%s/%s", item.GetNamespace(), item.GetName()),
			Item: record.ResourceMarshaller{Resource: anonymizeVirtualMachineInstances(item)},
		})
		// limit the gathered records
		if len(records) == limit {
			err = fmt.Errorf("limit %d for number of gathered %s resources exceeded (found: %d)",
				limit, virtualMachineInstancesResource.GroupResource(), len(virtualizationList.Items))
			errs = append(errs, err)
			break
		}
	}

	return records, errs
}

func anonymizeVirtualMachineInstances(data *unstructured.Unstructured) *unstructured.Unstructured {
	const errMsg = "error during anonymizing virtualmachineinstances:"
	volumes, err := utils.NestedSliceWrapper(data.Object, "spec", "volumes")
	if err != nil {
		klog.Infof("%s unable to find volumes %v", errMsg, err)
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
