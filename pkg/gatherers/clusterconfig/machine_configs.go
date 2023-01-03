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
)

// GatherMachineConfigs Collects MachineConfigs definitions. Following data is intentionally removed from the definitions:
// - `spec.config.storage.files`
// - `spec.config.passwd.users`
//
// ### API Reference
// - https://docs.openshift.com/container-platform/4.7/rest_api/machine_apis/machineconfig-machineconfiguration-openshift-io-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/machineconfigs/75-worker-sap-data-intelligence.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.8.2  | config/machineconfigs/{resource}.json 							|
//
// ### Config ID
// `clusterconfig/machine_configs`
//
// ### Released version
// - 4.8.2
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherMachineConfigs(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachineConfigs(ctx, gatherDynamicClient)
}

func gatherMachineConfigs(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	mcList, err := dynamicClient.Resource(machineConfigGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	records := []record.Record{}
	var errs []error
	for i := range mcList.Items {
		mc := mcList.Items[i]
		// remove the sensitive content by overwriting the values
		err := unstructured.SetNestedField(mc.Object, nil, "spec", "config", "storage", "files")
		if err != nil {
			klog.Errorf("unable to set nested field: %v", err)
			errs = append(errs, err)
		}
		err = unstructured.SetNestedField(mc.Object, nil, "spec", "config", "passwd", "users")
		if err != nil {
			klog.Errorf("unable to set nested field: %v", err)
			errs = append(errs, err)
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/machineconfigs/%s", mc.GetName()),
			Item: record.ResourceMarshaller{Resource: &mc},
		})
	}
	if len(errs) > 0 {
		return records, errs
	}
	return records, nil
}
