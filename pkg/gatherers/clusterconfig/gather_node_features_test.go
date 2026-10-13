package clusterconfig

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func Test_GatherNodeFeatures(t *testing.T) {
	tests := []struct {
		name                    string
		filePath                string
		expectedNumberOfRecords int
	}{
		{
			name:                    "no NodeFeature resource exists",
			filePath:                "",
			expectedNumberOfRecords: 0,
		},
		{
			name:                    "NodeFeature resource with all fields",
			filePath:                "testdata/nodefeature.yaml",
			expectedNumberOfRecords: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := createNodeFeatureDynamicClient()
			var resourceData []byte
			if tt.filePath != "" {
				var err error
				resourceData, err = os.ReadFile(tt.filePath)
				assert.NoError(t, err)
			}

			_, err := createNodeFeatureResource(ctx, client, resourceData)
			assert.NoError(t, err)
			records, errs := gatherNodeFeatures(ctx, client)
			assert.Nil(t, errs)
			assert.Len(t, records, tt.expectedNumberOfRecords)

			if tt.expectedNumberOfRecords > 0 {
				// Validate the record name format
				assert.Contains(t, records[0].Name, "namespaces/openshift-nfd/customresources/")

				// Marshal and validate the content
				marshaledRecord, err := records[0].Item.Marshal()
				assert.NoError(t, err)

				// Unmarshal into a NodeFeatureSpec to validate structure
				var recordSpec nfdv1alpha1.NodeFeatureSpec
				err = json.Unmarshal(marshaledRecord, &recordSpec)
				assert.NoError(t, err)

				// Verify only allowed attributes are present
				assert.Len(t, recordSpec.Features.Attributes, 2)
				assert.Contains(t, recordSpec.Features.Attributes, "cpu.topology")
				assert.Contains(t, recordSpec.Features.Attributes, "system.dmiid")

				// Verify specific values match expected output
				cpuTopology := recordSpec.Features.Attributes["cpu.topology"]
				assert.Equal(t, "true", cpuTopology.Elements["hardware_multithreading"])
				assert.Equal(t, "1", cpuTopology.Elements["socket_count"])

				systemDmiid := recordSpec.Features.Attributes["system.dmiid"]
				assert.Equal(t, "m6a.xlarge", systemDmiid.Elements["product_name"])
				assert.Equal(t, "Amazon EC2", systemDmiid.Elements["sys_vendor"])
			}
		})
	}
}

func Test_FilterNodeFeatureSpec(t *testing.T) {
	tests := []struct {
		name           string
		input          *nfdv1alpha1.NodeFeatureSpec
		expectedFields []string
	}{
		{
			name: "filters only allowed fields",
			input: &nfdv1alpha1.NodeFeatureSpec{
				Features: nfdv1alpha1.Features{
					Attributes: map[string]nfdv1alpha1.AttributeFeatureSet{
						"cpu.topology": {
							Elements: map[string]string{
								"hardware_multithreading": "true",
								"socket_count":            "1",
							},
						},
						"system.dmiid": {
							Elements: map[string]string{
								"product_name": "m6a.xlarge",
								"sys_vendor":   "Amazon EC2",
							},
						},
						"kernel.config": {
							Elements: map[string]string{
								"NO_HZ": "y",
							},
						},
					},
				},
			},
			expectedFields: []string{"cpu.topology", "system.dmiid"},
		},
		{
			name: "handles empty attributes",
			input: &nfdv1alpha1.NodeFeatureSpec{
				Features: nfdv1alpha1.Features{
					Attributes: map[string]nfdv1alpha1.AttributeFeatureSet{},
				},
			},
			expectedFields: []string{},
		},
		{
			name: "handles missing allowed fields",
			input: &nfdv1alpha1.NodeFeatureSpec{
				Features: nfdv1alpha1.Features{
					Attributes: map[string]nfdv1alpha1.AttributeFeatureSet{
						"kernel.config": {
							Elements: map[string]string{
								"NO_HZ": "y",
							},
						},
					},
				},
			},
			expectedFields: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterNodeFeatureSpec(tt.input)
			assert.NotNil(t, result)
			assert.Len(t, result.Features.Attributes, len(tt.expectedFields))
			for _, field := range tt.expectedFields {
				assert.Contains(t, result.Features.Attributes, field)
			}
		})
	}
}

func createNodeFeatureDynamicClient() dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		nodeFeatureResource: "NodeFeaturesList",
	})
}

func createNodeFeatureResource(
	ctx context.Context,
	client dynamic.Interface,
	resourceDefinition []byte,
) (unstructured.Unstructured, error) {
	if len(resourceDefinition) == 0 {
		return unstructured.Unstructured{}, nil
	}

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	resource := unstructured.Unstructured{}
	_, _, err := decUnstructured.Decode(resourceDefinition, nil, &resource)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	_, err = client.Resource(nodeFeatureResource).Namespace("openshift-nfd").Create(ctx, &resource, metav1.CreateOptions{})
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	return resource, nil
}
