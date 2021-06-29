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

// GatherMachineHealthCheck collects MachineHealthCheck information
//
// The Kubernetes api:
//       https://github.com/openshift/machine-api-operator/blob/master/pkg/generated/clientset/versioned/typed/machine/v1beta1/machinehealthcheck.go
// Response see:
//       https://docs.openshift.com/container-platform/4.3/rest_api/index.html#machinehealthcheck-v1beta1-machine-openshift-io
//
// * Location in archive: config/machinehealthchecks
// * Id in config: machine_healthchecks
// * Since versions:
//   * 4.8+
func (g *Gatherer) GatherMachineHealthCheck(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachineHealthCheck(ctx, dynamicClient)
}

func gatherMachineHealthCheck(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	machineHealthcheck, err := dynamicClient.Resource(machineHeatlhCheckGVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for _, i := range machineHealthcheck.Items {
		recordName := fmt.Sprintf("config/machinehealthchecks/%s", i.GetName())
		if i.GetNamespace() != "" {
			recordName = fmt.Sprintf("config/machinehealthchecks/%s/%s", i.GetNamespace(), i.GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: &i},
		})
	}

	return records, nil
}
