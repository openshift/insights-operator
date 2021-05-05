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

func Test_ContainerRuntimeConfig(t *testing.T) {
	var machineconfigpoolYAML = `
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
    name: test-ContainerRC
`
	gvr := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "containerruntimeconfigs"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "ContainerRuntimeConfigsList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testContainerRuntimeConfigs := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineconfigpoolYAML), nil, testContainerRuntimeConfigs)
	if err != nil {
		t.Fatal("unable to decode machineconfigpool ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testContainerRuntimeConfigs, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineconfigpool ", err)
	}

	ctx := context.Background()
	records, errs := gatherContainerRuntimeConfig(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "config/containerruntimeconfigs/test-ContainerRC" {
		t.Fatalf("unexpected containerruntimeconfig name %s", records[0].Name)
	}
}
