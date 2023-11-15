package clusterconfig

// nolint: dupl, lll

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// GatherMachineAutoscalers Collects `MachineAutoscalers` definition.
//
// ### API Reference
// - https://github.com/openshift/cluster-autoscaler-operator/blob/master/pkg/apis/autoscaling/v1beta1/machineautoscaler_types.go
// - https://docs.openshift.com/container-platform/4.7/rest_api/autoscale_apis/machineautoscaler-autoscaling-openshift-io-v1beta1.html#machineautoscaler-autoscaling-openshift-io-v1beta1
//
// ### Sample data
// - docs/insights-archive-sample/config/machineautoscalers/openshift-machine-api/worker-us-east-1a.json
//
// ### Location in archive
// - `config/machineautoscalers/{namespace}/{name}.json`
//
// ### Config ID
// `clusterconfig/machine_autoscalers`
//
// ### Released version
// - 4.8.2
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherMachineAutoscalers(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachineAutoscalers(ctx, dynamicClient)
}

func gatherMachineAutoscalers(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	machineAutoscaler, err := dynamicClient.Resource(machineAutoScalerGvr).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i, mas := range machineAutoscaler.Items {
		recordName := fmt.Sprintf("config/machineautoscalers/%s", mas.GetName())
		if mas.GetNamespace() != "" {
			recordName = fmt.Sprintf("config/machineautoscalers/%s/%s", mas.GetNamespace(), mas.GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: &machineAutoscaler.Items[i]},
		})
	}

	return records, nil
}
