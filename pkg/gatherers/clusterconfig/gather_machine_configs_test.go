package clusterconfig

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
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

func Test_getCRSize(t *testing.T) {
	tests := []struct {
		name          string
		machineConfig *unstructured.Unstructured
		expectError   bool
	}{
		{
			name: "machine config with spec",
			machineConfig: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfig",
					"metadata": map[string]interface{}{
						"name": "test-mc-with-spec",
					},
					"spec": map[string]interface{}{
						"config": map[string]interface{}{
							"ignition": map[string]interface{}{
								"version": "3.2.0",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:          "empty machine config",
			machineConfig: &unstructured.Unstructured{},
			expectError:   false,
		},
		{
			name: "failed to marshal machineconfig",
			machineConfig: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"invalid": make(chan int),
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, err := getCRSize(tt.machineConfig)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, 0, size)
			} else {
				// Verify the size calculation is correct by doing it ourselves
				expectedData, marshalErr := json.Marshal(tt.machineConfig)
				assert.NoError(t, marshalErr)
				assert.Equal(t, len(expectedData), size)
			}
		})
	}
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
			expectedNumberOfRecords: 1,
		},
		{
			name: "one machine config which is in use",
			machineConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: 75-worker-sap-data-intelligence`,
			},
			inUseMachineConfigs:     sets.Set[string]{"75-worker-sap-data-intelligence": {}},
			expectedNumberOfRecords: 2,
		},
		{
			name:                    "two machine configs but only one in use",
			expectedNumberOfRecords: 2,
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
			expectedNumberOfRecords: 1,
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
			expectedNumberOfRecords: 3,
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
		{
			name:                    "machine config has cr-size annotation",
			expectedNumberOfRecords: 2,
			machineConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  name: test-mc
spec:
  config:
    ignition:
      version: "3.2.0"
    storage:
      files:
        - path: /etc/test
          contents:
            source: data:,test
    passwd:
      users:
        - name: testuser`,
			},
			inUseMachineConfigs: sets.Set[string]{"test-mc": {}},
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

			// Verify cr-size annotation is present on machine config records
			for _, rec := range records {
				// Skip the aggregated count record
				if rec.Name == "aggregated/unused_machine_configs_count" {
					continue
				}

				// Extract the MachineConfig from the record
				mc, ok := rec.Item.(record.ResourceMarshaller)
				assert.True(t, ok, "record should be a ResourceMarshaller")

				unstructuredMC, ok := mc.Resource.(*unstructured.Unstructured)
				assert.True(t, ok, "resource should be an Unstructured")

				annotations := unstructuredMC.GetAnnotations()
				sizeStr, exists := annotations["insights.operator.openshift.io/cr-size"]
				assert.True(t, exists, "cr-size annotation should exist on %s", rec.Name)

				size, err := strconv.Atoi(sizeStr)
				assert.NoError(t, err, "cr-size should be a valid integer")
				assert.Greater(t, size, 0, "cr-size should be positive")

				// Verify that sensitive fields were removed (if they existed)
				files, _, _ := unstructured.NestedFieldNoCopy(unstructuredMC.Object, "spec", "config", "storage", "files")
				assert.Nil(t, files, "storage.files should be removed")

				users, _, _ := unstructured.NestedFieldNoCopy(unstructuredMC.Object, "spec", "config", "passwd", "users")
				assert.Nil(t, users, "passwd.users should be removed")
			}
		})
	}
}
