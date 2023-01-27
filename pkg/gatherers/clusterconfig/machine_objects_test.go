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

func Test_MachineObject_Gather(t *testing.T) {
	var machineObjectYAML = `
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: test-master
`
	gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machines"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "MachineList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testMachineObject := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineObjectYAML), nil, testMachineObject)
	if err != nil {
		t.Fatal("unable to decode machineobject ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testMachineObject, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineobject ", err)
	}

	ctx := context.Background()
	records, errs := gatherMachineObject(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number of records %d", len(records))
	}
	if records[0].Name != "machineobjects/test-master" {
		t.Fatalf("unexpected machineobject name %s", records[0].Name)
	}
}
