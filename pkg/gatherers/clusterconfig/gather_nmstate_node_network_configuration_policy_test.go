// nolint: dupl
package clusterconfig

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGatherNodeNetworkConfigurationPolicy(t *testing.T) {
	tests := []struct {
		name                    string
		filePath                string
		expectedErrors          []error
		expectedNumberOfRecords int
	}{
		{
			name:                    "no NodeNetworkConfigurationPolicy exists",
			filePath:                "",
			expectedErrors:          nil,
			expectedNumberOfRecords: 0,
		},
		{
			name:                    "NodeNetworkConfigurationPolicy exists",
			filePath:                "testdata/node_network_configuration_policy.yaml",
			expectedErrors:          nil,
			expectedNumberOfRecords: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cli := createDynamicClient(nodeNetConfPoliciesV1GVR, "NodeNetworkConfigurationPoliciesList")
			var resourceData []byte
			if tt.filePath != "" {
				var err error
				resourceData, err = os.ReadFile(tt.filePath)
				assert.NoError(t, err)
			}

			unstructuredResource, err := createResource(ctx, cli, resourceData, nodeNetConfPoliciesV1GVR)
			assert.NoError(t, err)
			records, errs := gatherNodeNetworkConfigurationPolicy(ctx, cli)
			assert.Equal(t, tt.expectedErrors, errs)
			assert.Len(t, records, tt.expectedNumberOfRecords)
			if tt.expectedNumberOfRecords > 0 {
				marshaledRecord, err := records[0].Item.Marshal()
				assert.NoError(t, err)
				unstructuredRec := unstructured.Unstructured{}
				err = json.Unmarshal(marshaledRecord, &unstructuredRec)
				assert.NoError(t, err)
				assert.Equal(t, unstructuredResource, unstructuredRec)
			}
		})
	}
}
