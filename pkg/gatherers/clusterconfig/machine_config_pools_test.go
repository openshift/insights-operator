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

func Test_MachineConfigPool_Gather(t *testing.T) {
	var machineconfigpoolYAML = `
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigPool
metadata:
    name: master-t
`
	gvr := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "MachineConfigPoolsList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testMachineConfigPools := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineconfigpoolYAML), nil, testMachineConfigPools)
	if err != nil {
		t.Fatal("unable to decode machineconfigpool ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testMachineConfigPools, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineconfigpool ", err)
	}

	ctx := context.Background()
	records, errs := gatherMachineConfigPool(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "config/machineconfigpools/master-t" {
		t.Fatalf("unexpected machineconfigpool name %s", records[0].Name)
	}
}
