package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherControlPlaneMachineSet Collects `ControlPlaneMachineSet` information.
//
// ### API Reference
// - https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/machine_apis/controlplanemachineset-machine-openshift-io-v1
//
// ### Sample data
// - docs/insights-archive-sample/config/controlplanemachinesets/openshift-machine-api/cluster.json
//
// ### Location in archive
// - `config/controlplanemachinesets/{resource}`
// - `config/controlplanemachinesets/{namespace}/{resource}`
//
// ### Config ID
// `clusterconfig/control_plane_machine_sets`
//
// ### Released version
// - 4.23.0
//
// ### Backported versions
// - 4.19
//
// ### Changes
// None
func (g *Gatherer) GatherControlPlaneMachineSet(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherControlPlaneMachineSet(ctx, dynamicClient)
}

func gatherControlPlaneMachineSet(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	controlPlaneMachineSets, err := dynamicClient.Resource(controlPlaneMachineSetVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var errs []error
	var records []record.Record
	for _, ms := range controlPlaneMachineSets.Items {
		recordName := fmt.Sprintf("config/controlplanemachinesets/%s", ms.GetName())
		if ms.GetNamespace() != "" {
			recordName = fmt.Sprintf("config/controlplanemachinesets/%s/%s", ms.GetNamespace(), ms.GetName())
		}

		// remove the sensitive content by overwriting the values
		err = unstructured.SetNestedField(ms.Object, nil, "spec", "templates")
		if err != nil {
			klog.Errorf("unable to set nested field: %v", err)
			errs = append(errs, err)
		}

		err = unstructured.SetNestedField(ms.Object, nil, "spec", "template")
		if err != nil {
			klog.Errorf("unable to set nested field: %v", err)
			errs = append(errs, err)
		}

		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: &ms},
		})
	}

	return records, errs
}
