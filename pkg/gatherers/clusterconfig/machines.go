package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherMachine collects Machine information
//
// The Kubernetes api:
//
//	https://github.com/openshift/api/blob/master/machine/v1beta1/types_machine.go
//
// Response see:
//
//	https://docs.openshift.com/container-platform/4.12/rest_api/machine_apis/machine-machine-openshift-io-v1beta1.html
//
// * Location in archive: config/machines/
// * Id in config: clusterconfig/machines
// * Since versions:
//   - 4.13+
func (g *Gatherer) GatherMachine(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMachine(ctx, dynamicClient)
}

func gatherMachine(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machines"}
	machines, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
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
