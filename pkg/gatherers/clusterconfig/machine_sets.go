package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherMachineSet collects MachineSet information
//
// The Kubernetes api:
//       https://github.com/openshift/machine-api-operator/blob/master/pkg/generated/clientset/versioned/typed/machine/v1beta1/machineset.go
// Response see:
//       https://docs.openshift.com/container-platform/4.3/rest_api/index.html#machineset-v1beta1-machine-openshift-io
//
// * Location in archive: machinesets/
// * Id in config: machine_sets
// * Since versions:
//   * 4.4.29+
//   * 4.5.15+
//   * 4.6+
func (g *Gatherer) GatherMachineSet(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachineSet(ctx, dynamicClient)
}

func gatherMachineSet(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinesets"}
	machineSets, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i, ms := range machineSets.Items {
		recordName := fmt.Sprintf("machinesets/%s", ms.GetName())
		if ms.GetNamespace() != "" {
			recordName = fmt.Sprintf("machinesets/%s/%s", ms.GetNamespace(), ms.GetName())
		}
		records = append(records, record.Record{
			Name: recordName,
			Item: record.ResourceMarshaller{Resource: anonymizeMachineset(&machineSets.Items[i])},
		})
	}

	return records, nil
}

func anonymizeMachineset(data *unstructured.Unstructured) *unstructured.Unstructured {
	fieldsToAnonymize := [][]string{
		{"spec", "template", "spec", "providerSpec", "value", "projectID"},
		{"spec", "template", "spec", "providerSpec", "value", "region"},
		{"spec", "template", "spec", "providerSpec", "value", "placement", "availabilityZone"},
		{"spec", "template", "spec", "providerSpec", "value", "placement", "region"},
	}

	for _, fieldToAnonymize := range fieldsToAnonymize {
		err := anonymize.UnstructuredNestedStringField(data.Object, fieldToAnonymize...)
		if err != nil {
			klog.Infof("error during anonymizing machineset: %v", err)
		}
	}

	return data
}
