//nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// GatherMachineAutoscalers collects MachineAutoscalers definition
//
// The Kubernetes api:
//       https://github.com/openshift/cluster-autoscaler-operator/blob/master/pkg/apis/autoscaling/v1beta1/machineautoscaler_types.go
// Response see:
//       https://docs.openshift.com/container-platform/4.7/rest_api/autoscale_apis/machineautoscaler-autoscaling-openshift-io-v1beta1.html#machineautoscaler-autoscaling-openshift-io-v1beta1
//
// * Location in archive: config/machineautoscalers/{namespace}/{machineautoscaler-name}.json
// * Id in config: machine_autoscalers
// * Since versions:
//   * 4.8+
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
	for _, i := range machineAutoscaler.Items {
		recordName := fmt.Sprintf("config/machineautoscalers/%s", i.GetName())
		if i.GetNamespace() != "" {
			recordName = fmt.Sprintf("config/machineautoscalers/%s/%s", i.GetNamespace(), i.GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: &i},
		})
	}

	return records, nil
}
