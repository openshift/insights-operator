// nolint: dupl
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
)

func TestGatherNodeNetworkState(t *testing.T) {
	tests := []struct {
		name                    string
		filePath                string
		expectedErrors          []error
		expectedNumberOfRecords int
		obfuscated              bool
	}{
		{
			name:                    "no NodeNetworkState resource exists",
			filePath:                "",
			expectedErrors:          []error{},
			expectedNumberOfRecords: 0,
		},
		{
			name:                    "NodeNetworkState resource with no interfaces",
			filePath:                "testdata/node_network_state_no_interfaces.yaml",
			expectedErrors:          []error{},
			expectedNumberOfRecords: 1,
		},
		{
			name:                    "NodeNetworkState resource with interfaces",
			filePath:                "testdata/node_network_state_with_interfaces.yaml",
			expectedErrors:          []error{},
			expectedNumberOfRecords: 1,
		},
		{
			name:                    "NodeNetworkState resource with interfaces with some mac addresses",
			filePath:                "testdata/node_network_state_with_interfaces_and_mac_addresses.yaml",
			expectedErrors:          []error{},
			expectedNumberOfRecords: 1,
			obfuscated:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := createDynamicClient(nodeNetStatesV1Beta1GVR, "NodeNetworkStatesList")
			var resourceData []byte
			if tt.filePath != "" {
				var err error
				resourceData, err = os.ReadFile(tt.filePath)
				assert.NoError(t, err)
			}

			unstructuredResource, err := createResource(ctx, client, resourceData, nodeNetStatesV1Beta1GVR)
			assert.NoError(t, err)
			records, errs := gatherNodeNetworkState(ctx, client)
			assert.Equal(t, tt.expectedErrors, errs)
			assert.Len(t, records, tt.expectedNumberOfRecords)

			if tt.expectedNumberOfRecords > 0 {
				marshaledRecord, err := records[0].Item.Marshal()
				assert.NoError(t, err)
				unstructuredRec := unstructured.Unstructured{}
				err = json.Unmarshal(marshaledRecord, &unstructuredRec)
				assert.NoError(t, err)
				if tt.obfuscated {
					err := anonymizeNodeNetworkState(unstructuredResource.Object)
					assert.NoError(t, err)
					assert.Equal(t, unstructuredResource, unstructuredRec)
				} else {
					assert.Equal(t, unstructuredResource, unstructuredRec)
				}
			}
		})
	}
}

func createDynamicClient(gvr schema.GroupVersionResource, gvrList string) dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: gvrList,
	})
}

func createResource(ctx context.Context, client dynamic.Interface, resourceDefinition []byte,
	gvr schema.GroupVersionResource) (unstructured.Unstructured, error) {
	if len(resourceDefinition) == 0 {
		return unstructured.Unstructured{}, nil
	}

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	resource := unstructured.Unstructured{}
	_, _, err := decUnstructured.Decode(resourceDefinition, nil, &resource)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	_, err = client.Resource(gvr).Create(ctx, &resource, metav1.CreateOptions{})
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	return resource, nil
}
