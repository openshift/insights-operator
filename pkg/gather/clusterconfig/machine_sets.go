package clusterconfig

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

//GatherMachineSet collects MachineSet information
//
// The Kubernetes api https://github.com/openshift/machine-api-operator/blob/master/pkg/generated/clientset/versioned/typed/machine/v1beta1/machineset.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#machineset-v1beta1-machine-openshift-io
//
// Location in archive: machinesets/
func GatherMachineSet(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinesets"}
		machineSets, err := g.dynamicClient.Resource(gvr).List(g.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, i := range machineSets.Items {
			recordName := fmt.Sprintf("machinesets/%s", i.GetName())
			if i.GetNamespace() != "" {
				recordName = fmt.Sprintf("machinesets/%s/%s", i.GetNamespace(), i.GetName())
			}
			records = append(records, record.Record{
				Name: recordName,
				Item: record.JSONMarshaller{Object: i.Object},
			})
		}
		return records, nil
	}
}
