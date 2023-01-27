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

func Test_Machine_Gather(t *testing.T) {
	var machineYAML = `
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

	testMachine := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineYAML), nil, testMachine)
	if err != nil {
		t.Fatal("unable to decode machine ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testMachine, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machine ", err)
	}

	ctx := context.Background()
	records, errs := gatherMachine(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number of records %d", len(records))
	}
	if records[0].Name != "machines/test-master" {
		t.Fatalf("unexpected machine name %s", records[0].Name)
	}
}
