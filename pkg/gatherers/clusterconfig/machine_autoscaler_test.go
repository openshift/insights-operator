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

func Test_MachineAutoScaler_Gather(t *testing.T) {
	var masYAML = `
apiVersion: autoscaling.openshift.io/v1beta1
kind: MachineAutoscaler
metadata:
    name: test-autoscaler
`

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		machineAutoScalerGvr: "MachineAutoScalerList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testMachineAutoScaler := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(masYAML), nil, testMachineAutoScaler)
	if err != nil {
		t.Fatal("unable to decode machineautoscaler ", err)
	}
	_, err = client.Resource(machineAutoScalerGvr).Create(context.Background(), testMachineAutoScaler, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineautoscaler ", err)
	}

	ctx := context.Background()
	records, errs := gatherMachineAutoscalers(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "config/machineautoscalers/test-autoscaler" {
		t.Fatalf("unexpected machineautoscaler name %s", records[0].Name)
	}
}
