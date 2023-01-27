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
	for i, ms := range machines.Items {
		recordName := fmt.Sprintf("machines/%s", ms.GetName())
		if ms.GetNamespace() != "" {
			recordName = fmt.Sprintf("machines/%s/%s", ms.GetNamespace(), ms.GetName())
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
