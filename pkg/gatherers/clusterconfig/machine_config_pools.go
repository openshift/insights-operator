//nolint: dupl
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

// GatherMachineConfigPool collects MachineConfigPool information
//
// The Kubernetes api:
//     https://github.com/openshift/machine-config-operator/blob/master/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L197
// Response see:
//     https://docs.okd.io/latest/rest_api/machine_apis/machineconfigpool-machineconfiguration-openshift-io-v1.html
//
// * Location in archive: config/machineconfigpools/
// * Id in config: machine_config_pools
// * Since versions:
//   * 4.5.33+
//   * 4.6+
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
	for _, i := range machineCPs.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/machineconfigpools/%s", i.GetName()),
			Item: record.ResourceMarshaller{Resource: &i},
		})
	}

	return records, nil
}
