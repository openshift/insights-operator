// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherMachineConfigPool Collects MachineConfigPool information.
//
// ### API Reference
// - https://github.com/openshift/machine-config-operator/blob/master/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L197
// - https://docs.okd.io/latest/rest_api/machine_apis/machineconfigpool-machineconfiguration-openshift-io-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/machineconfigpools
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.7.0  | config/machineconfigpools 									|
//
// ### Config ID
// `clusterconfig/machine_config_pools`
//
// ### Released version
// - 4.7.0
//
// ### Backported versions
// - 4.5.33+
// - 4.6.16+
//
// ### Changes
// None
func (g *Gatherer) GatherMachineConfigPool(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachineConfigPool(ctx, dynamicClient)
}

func gatherMachineConfigPool(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	mcp := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"}
	machineCPs, err := dynamicClient.Resource(mcp).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i, mcp := range machineCPs.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/machineconfigpools/%s", mcp.GetName()),
			Item: record.ResourceMarshaller{Resource: &machineCPs.Items[i]},
		})
	}

	return records, nil
}
