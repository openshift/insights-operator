package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherMachine Collects `Machine` information.
//
// ### API Reference
// - https://github.com/openshift/api/blob/master/machine/v1beta1/types_machine.go
// - https://docs.openshift.com/container-platform/4.12/rest_api/machine_apis/machine-machine-openshift-io-v1beta1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/machines/openshift-machine-api/
//
// ### Location in archive
// - `config/machines/`
//
// ### Config ID
// `clusterconfig/machines`
//
// ### Released version
// - 4.13.0
//
// ### Backported versions
// - 4.11.29+
// - 4.12.5+
//
// ### Changes
// None
func (g *Gatherer) GatherMachine(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachine(ctx, dynamicClient)
}

func gatherMachine(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	machines, err := dynamicClient.Resource(machinesGVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var records []record.Record
	for i := range machines.Items {
		recordName := fmt.Sprintf("config/machines/%s", machines.Items[i].GetName())
		if machines.Items[i].GetNamespace() != "" {
			recordName = fmt.Sprintf("config/machines/%s/%s", machines.Items[i].GetNamespace(), machines.Items[i].GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: anonymizeMachine(&machines.Items[i])},
		})
	}

	return records, nil
}

func anonymizeMachine(data *unstructured.Unstructured) *unstructured.Unstructured {
	fieldsToAnonymize := [][]string{
		{"spec", "providerID"},
		{"spec", "providerSpec", "value", "placement", "availabilityZone"},
		{"spec", "providerSpec", "value", "placement", "region"},
		{"metadata", "labels", "machine.openshift.io/region"},
	}

	for _, fieldToAnonymize := range fieldsToAnonymize {
		err := anonymize.UnstructuredNestedStringField(data.Object, fieldToAnonymize...)
		if err != nil {
			klog.Infof("error during anonymizing machine: %v", err)
		}
	}

	return data
}
