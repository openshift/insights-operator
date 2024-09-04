package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func createMockConfigMachine(ctx context.Context, c dynamic.Interface, data string) error {
	decUnstructured1 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testMachineConfig := &unstructured.Unstructured{}
	_, _, err := decUnstructured1.Decode([]byte(data), nil, testMachineConfig)
	if err != nil {
		return err
	}

	_, err = c.
		Resource(machineConfigGroupVersionResource).
		Create(ctx, testMachineConfig, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func TestGatherMachineConfigs(t *testing.T) {
	tests := []struct {
		name                    string
		machineConfigYAMLs      []string
		inUseMachineConfigs     sets.Set[string]
		expectedNumberOfRecords int
	}{
		{
			name:                    "no machine configs exists",
			machineConfigYAMLs:      []string{},
			inUseMachineConfigs:     sets.Set[string]{},
			expectedNumberOfRecords: 0,
		},
		{
			name: "one machine config which is in use",
			machineConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1 
kind: MachineConfig 
metadata: 
  name: 75-worker-sap-data-intelligence`},
			inUseMachineConfigs:     sets.Set[string]{"75-worker-sap-data-intelligence": {}},
			expectedNumberOfRecords: 1,
		},
		{
			name:                    "two machine configs but only one in use",
			expectedNumberOfRecords: 1,
			machineConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: test-not-in-use`,
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: in-use`,
			},
			inUseMachineConfigs: sets.Set[string]{"in-use": {}},
		},
		{
			name:                    "no machine config in use",
			expectedNumberOfRecords: 0,
			machineConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: test-not-in-use-1`,
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: test-not-in-use-2`,
			},
			inUseMachineConfigs: sets.Set[string]{"in-use": {}, "in-use-2": {}},
		},
		{
			name:                    "two machine configs in use",
			expectedNumberOfRecords: 2,
			machineConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: in-use-1`,
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: in-use-2`,
			},
			inUseMachineConfigs: sets.Set[string]{"in-use-1": {}, "in-use-2": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Initialize the fake dynamic client.
			machineConfigClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				machineConfigGroupVersionResource: "MachineConfigsList",
			})
			for _, mcYAML := range tt.machineConfigYAMLs {
				err := createMockConfigMachine(ctx, machineConfigClient, mcYAML)
				assert.NoError(t, err)
			}
			records, errs := gatherMachineConfigs(ctx, machineConfigClient, tt.inUseMachineConfigs)
			assert.Len(t, errs, 0)
			assert.Len(t, records, tt.expectedNumberOfRecords)
		})
	}
}
