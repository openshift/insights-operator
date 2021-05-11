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

func Test_SAPMachineConfigs(t *testing.T) {
	// Initialize the fake dynamic client.
	machineConfigYAML1 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 75-worker-sap-data-intelligence
`
	machineConfigYAML2 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 75-master-sap-data-intelligence
`
	machineConfigYAML3 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 99-sdi-generated-containerruntime
    ownerReferences:
        - kind: ContainerRuntimeConfig
          name: sdi-pids-limit
`
	machineConfigYAML4 := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
    name: 00-example-machine-config
    labels:
        workload: sap-data-intelligence
`

	machineConfigClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		machineConfigGroupVersionResource: "MachineConfigsList",
	})

	decUnstructured1 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testMachineConfig1 := &unstructured.Unstructured{}
	_, _, err := decUnstructured1.Decode([]byte(machineConfigYAML1), nil, testMachineConfig1)
	if err != nil {
		t.Fatal("unable to decode MachineConfig 1 YAML", err)
	}

	decUnstructured2 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testMachineConfig2 := &unstructured.Unstructured{}
	_, _, err = decUnstructured2.Decode([]byte(machineConfigYAML2), nil, testMachineConfig2)
	if err != nil {
		t.Fatal("unable to decode MachineConfig 2 YAML", err)
	}

	decUnstructured3 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testMachineConfig3 := &unstructured.Unstructured{}
	_, _, err = decUnstructured3.Decode([]byte(machineConfigYAML3), nil, testMachineConfig3)
	if err != nil {
		t.Fatal("unable to decode MachineConfig 3 YAML", err)
	}

	decUnstructured4 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testMachineConfig4 := &unstructured.Unstructured{}
	_, _, err = decUnstructured4.Decode([]byte(machineConfigYAML4), nil, testMachineConfig4)
	if err != nil {
		t.Fatal("unable to decode MachineConfig 4 YAML", err)
	}

	records, errs := gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 0 records because there is no MachineConfigs yet.
	if len(records) != 0 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}

	// Create first MachineConfig resource.
	_, _ = machineConfigClient.
		Resource(machineConfigGroupVersionResource).
		Create(context.Background(), testMachineConfig1, metav1.CreateOptions{})

	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 1 record because there is now 1 MachineConfig resource.
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}

	// Create second MachineConfig resource.
	_, _ = machineConfigClient.
		Resource(machineConfigGroupVersionResource).
		Create(context.Background(), testMachineConfig2, metav1.CreateOptions{})

	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 2 record because there are now 2 MachineConfig resource.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the third run: %d", len(records))
	}

	// Create third MachineConfig resource.
	_, _ = machineConfigClient.
		Resource(machineConfigGroupVersionResource).
		Create(context.Background(), testMachineConfig3, metav1.CreateOptions{})

	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 3 record because there are now 3 MachineConfig resource.
	if len(records) != 3 {
		t.Fatalf("unexpected number or records in the fourth run: %d", len(records))
	}

	// Create fourth MachineConfig resource.
	_, _ = machineConfigClient.
		Resource(machineConfigGroupVersionResource).
		Create(context.Background(), testMachineConfig4, metav1.CreateOptions{})

	records, errs = gatherSAPMachineConfigs(context.Background(), machineConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 4 record because there are now 4 MachineConfig resource.
	if len(records) != 4 {
		t.Fatalf("unexpected number or records in the fifth run: %d", len(records))
	}
}
