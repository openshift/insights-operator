package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func createMockKubeletConfig(ctx context.Context, dynamicCli *dynamicfake.FakeDynamicClient, yamlDefinition string) error {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	unstructuredObj := &unstructured.Unstructured{}
	_, _, err := decUnstructured.Decode([]byte(yamlDefinition), nil, unstructuredObj)
	if err != nil {
		return err
	}

	_, err = dynamicCli.
		Resource(kubeletGroupVersionResource).
		Create(ctx, unstructuredObj, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func Test_GatherKubeletConfig(t *testing.T) {
	tests := []struct {
		name                    string
		kubeletConfigYAMLs      []string
		expectedNumberOfRecords int
		expectedRecordNames     []string
	}{
		{
			name:                    "no kubeletconfigs exist",
			kubeletConfigYAMLs:      []string{},
			expectedNumberOfRecords: 0,
			expectedRecordNames:     []string{},
		},
		{
			name: "multiple kubeletconfigs",
			kubeletConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: set-max-pods
spec:
  kubeletConfig:
    maxPods: 100
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ""`,
				`apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: custom-config
spec:
  kubeletConfig:
    systemReserved:
      cpu: 500m
      memory: 512Mi
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/master: ""`,
			},
			expectedNumberOfRecords: 2,
			expectedRecordNames: []string{
				"config/kubeletconfigs/set-max-pods",
				"config/kubeletconfigs/custom-config",
			},
		},
		{
			name: "kubeletconfig with complex configuration",
			kubeletConfigYAMLs: []string{
				`apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: complex-config
  annotations:
    machineconfiguration.openshift.io/mc-name-suffix: ""
spec:
  kubeletConfig:
    maxPods: 250
    systemReserved:
      cpu: "1000m"
      memory: "1Gi"
      ephemeral-storage: "1Gi"
    kubeReserved:
      cpu: "500m"
      memory: "512Mi"
    evictionHard:
      memory.available: "500Mi"
      nodefs.available: "10%"
      nodefs.inodesFree: "5%"
      imagefs.available: "15%"
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ""`,
			},
			expectedNumberOfRecords: 1,
			expectedRecordNames:     []string{"config/kubeletconfigs/complex-config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Initialize the fake dynamic client
			kubeletConfigClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				runtime.NewScheme(),
				map[schema.GroupVersionResource]string{
					kubeletGroupVersionResource: "KubeletConfigList",
				},
			)

			// Create mock kubeletconfigs
			for _, kcYAML := range tt.kubeletConfigYAMLs {
				err := createMockKubeletConfig(ctx, kubeletConfigClient, kcYAML)
				assert.NoError(t, err)
			}

			records, errs := gatherGatherKubeletConfig(ctx, kubeletConfigClient)

			// Verify no errors
			assert.Len(t, errs, 0)

			// Verify number of records
			assert.Len(t, records, tt.expectedNumberOfRecords)

			// Verify record names
			actualRecordNames := make([]string, len(records))
			for i, rec := range records {
				actualRecordNames[i] = rec.Name
			}
			assert.ElementsMatch(t, tt.expectedRecordNames, actualRecordNames)

			// Verify each record contains valid data
			for _, rec := range records {
				assert.NotNil(t, rec.Item)

				// Verify the record item is of correct type
				jsonMarshaller, ok := rec.Item.(record.JSONMarshaller)
				assert.True(t, ok, "record item should be JSONMarshaller")
				assert.NotNil(t, jsonMarshaller.Object, "JSONMarshaller should have an Object")
			}
		})
	}
}

func Test_GatherKubeletConfig_RecordStructure(t *testing.T) {
	ctx := context.Background()
	kubeletConfigYAML := `apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: test-config
  annotations:
    test-annotation: "test-value"
spec:
  kubeletConfig:
    maxPods: 150
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ""`

	kubeletConfigClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			kubeletGroupVersionResource: "KubeletConfigList",
		},
	)

	err := createMockKubeletConfig(ctx, kubeletConfigClient, kubeletConfigYAML)
	assert.NoError(t, err)

	records, errs := gatherGatherKubeletConfig(ctx, kubeletConfigClient)
	assert.Len(t, errs, 0)
	assert.Len(t, records, 1)

	rec := records[0]

	// Verify record name format
	assert.Equal(t, "config/kubeletconfigs/test-config", rec.Name)

	// Verify record structure
	jsonMarshaller, ok := rec.Item.(record.JSONMarshaller)
	assert.True(t, ok)

	unstructuredObj, ok := jsonMarshaller.Object.(map[string]interface{})
	assert.True(t, ok)

	// Verify basic fields
	assert.Equal(t, "machineconfiguration.openshift.io/v1", unstructuredObj["apiVersion"])
	assert.Equal(t, "KubeletConfig", unstructuredObj["kind"])

	// Verify metadata
	metadata, ok := unstructuredObj["metadata"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "test-config", metadata["name"])

	// Verify annotations are preserved
	annotations, ok := metadata["annotations"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "test-value", annotations["test-annotation"])

	// Verify spec
	spec, ok := unstructuredObj["spec"].(map[string]interface{})
	assert.True(t, ok)

	kubeletConfig, ok := spec["kubeletConfig"].(map[string]interface{})
	assert.True(t, ok)

	maxPods, ok := kubeletConfig["maxPods"].(int64)
	assert.True(t, ok)
	assert.Equal(t, int64(150), maxPods)
}
