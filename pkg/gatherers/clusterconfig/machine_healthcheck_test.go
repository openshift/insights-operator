//nolint: dupl
package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_MachineHealthCheck_Gather(t *testing.T) {
	var machineHealthCheckYAML = `
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
    name: test-machinehealthcheck
`
	gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinehealthchecks"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "MachineHeachCheckList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testMachineHealthCheck := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineHealthCheckYAML), nil, testMachineHealthCheck)
	if err != nil {
		t.Fatal("unable to decode machinehealthcheck ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testMachineHealthCheck, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machinehealthcheck ", err)
	}

	ctx := context.Background()
	records, errs := gatherMachineHealthCheck(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "config/machinehealthchecks/test-machinehealthcheck" {
		t.Fatalf("unexpected machinehealthcheck name %s", records[0].Name)
	}
}
