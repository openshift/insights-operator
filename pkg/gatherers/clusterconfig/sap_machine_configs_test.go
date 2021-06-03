package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func createMockConfigMachine(t *testing.T, c dynamic.Interface, data string) {
	decUnstructured1 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testMachineConfig := &unstructured.Unstructured{}
	_, _, err := decUnstructured1.Decode([]byte(data), nil, testMachineConfig)
	if err != nil {
		t.Fatal("unable to decode MachineConfig YAML", err)
	}

	_, _ = c.
		Resource(machineConfigGroupVersionResource).
		Create(context.Background(), testMachineConfig, metav1.CreateOptions{})
}

func Test_SAPMachineConfigs(t *testing.T) {
	// Initialize the fake dynamic client.
	machineConfigClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		machineConfigGroupVersionResource: "MachineConfigsList",
	})

	records, errs := gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 0 records because there is no MachineConfigs yet.
	if len(records) != 0 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}

	// Create first MachineConfig resource.
	machineConfigYAML1 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 75-worker-sap-data-intelligence
`

	createMockConfigMachine(t, machineConfigClient, machineConfigYAML1)
	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 1 record because there is now 1 MachineConfig resource.
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}

	// Create second MachineConfig resource.
	machineConfigYAML2 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 75-master-sap-data-intelligence
`

	createMockConfigMachine(t, machineConfigClient, machineConfigYAML2)
	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 2 record because there are now 2 MachineConfig resource.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the third run: %d", len(records))
	}

	// Create third MachineConfig resource.
	machineConfigYAML3 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 99-sdi-generated-containerruntime
    ownerReferences:
        - kind: ContainerRuntimeConfig
          name: sdi-pids-limit
`

	createMockConfigMachine(t, machineConfigClient, machineConfigYAML3)
	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 3 record because there are now 3 MachineConfig resource.
	if len(records) != 3 {
		t.Fatalf("unexpected number or records in the fourth run: %d", len(records))
	}

	// Create fourth MachineConfig resource.
	machineConfigYAML4 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 00-example-machine-config
    labels:
        workload: sap-data-intelligence
`

	createMockConfigMachine(t, machineConfigClient, machineConfigYAML4)
	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 4 record because there are now 4 MachineConfig resource.
	if len(records) != 4 {
		t.Fatalf("unexpected number or records in the fifth run: %d", len(records))
	}
}
