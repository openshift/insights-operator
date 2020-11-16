package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

//GatherMachineConfigPool collects MachineConfigPool information
//
// The Kubernetes api https://github.com/openshift/machine-config-operator/blob/master/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L197
// Response see https://docs.okd.io/latest/rest_api/machine_apis/machineconfigpool-machineconfiguration-openshift-io-v1.html
//
// Location in archive: config/machineconfigpools/
func GatherMachineConfigPool(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
		if err != nil {
			return nil, []error{err}
		}
		return gatherMachineConfigPool(g.ctx, dynamicClient)
	}
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
	records := []record.Record{}
	for _, i := range machineCPs.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/machineconfigpools/%s", i.GetName()),
			Item: record.JSONMarshaller{Object: i.Object},
		})
	}
	return records, nil
}
