// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// GatherMachineHealthCheck Collects `MachineHealthCheck` information.
//
// ### API Reference
// - https://github.com/openshift/api/blob/master/machine/v1beta1/types_machinehealthcheck.go
// - https://docs.openshift.com/container-platform/4.3/rest_api/index.html#machinehealthcheck-v1beta1-machine-openshift-io
//
// ### Sample data
// - docs/insights-archive-sample/config/machinehealthchecks/openshift-machine-api/machine-api-termination-handler.json
//
// ### Location in archive
// - `config/machinehealthchecks/{namespace}/{resource}.json`
//
// ### Config ID
// `clusterconfig/machine_healthchecks`
//
// ### Released version
// - 4.8.0
//
// ### Backported versions
// None
//
// ### Changes
// None
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
	for i, mhc := range machineHealthcheck.Items {
		recordName := fmt.Sprintf("config/machinehealthchecks/%s", mhc.GetName())
		if mhc.GetNamespace() != "" {
			recordName = fmt.Sprintf("config/machinehealthchecks/%s/%s", mhc.GetNamespace(), mhc.GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: &machineHealthcheck.Items[i]},
		})
	}

	return records, nil
}
