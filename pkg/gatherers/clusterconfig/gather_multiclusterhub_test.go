package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func Test_gatherMultiClusterHub(t *testing.T) {
	multiClusterHubYAML := `
apiVersion: operator.open-cluster-management.io/v1
kind: MultiClusterHub
metadata:
  name: multiclusterhub
spec:
  availabilityConfig: High
  disableHubSelfManagement: false
  enableClusterBackup: false
status:
  currentVersion: 2.10.0
  desiredVersion: 2.10.0
  phase: Running
`

	t.Run("successfully gathers MultiClusterHub", func(t *testing.T) {
		dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			multiClusterHubGVR: "MultiClusterHubList",
		})
		decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		testHub := &unstructured.Unstructured{}
		_, _, err := decUnstructured.Decode([]byte(multiClusterHubYAML), nil, testHub)
		assert.NoError(t, err)

		_, err = dynamicClient.Resource(multiClusterHubGVR).Create(context.Background(), testHub, metav1.CreateOptions{})
		assert.NoError(t, err)

		records, errs := gatherMultiClusterHub(context.Background(), dynamicClient)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)
		assert.Equal(t,
			"cluster-scoped-resources/operator.open-cluster-management.io/multiclusterhubs/multiclusterhub",
			records[0].Name,
		)

		recordData, err := records[0].Item.Marshal()
		assert.NoError(t, err)
		var gathered map[string]interface{}
		err = json.Unmarshal(recordData, &gathered)
		assert.NoError(t, err)

		availabilityConfig, ok, err := unstructured.NestedString(gathered, "spec", "availabilityConfig")
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "High", availabilityConfig)

		phase, ok, err := unstructured.NestedString(gathered, "status", "phase")
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "Running", phase)
	})

	t.Run("returns nil when no MultiClusterHub exists", func(t *testing.T) {
		dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			multiClusterHubGVR: "MultiClusterHubList",
		})

		records, errs := gatherMultiClusterHub(context.Background(), dynamicClient)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("returns error when List fails", func(t *testing.T) {
		dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			multiClusterHubGVR: "MultiClusterHubList",
		})
		dynamicClient.PrependReactor("list", "multiclusterhubs", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("connection refused")
		})

		records, errs := gatherMultiClusterHub(context.Background(), dynamicClient)
		assert.Nil(t, records)
		assert.Len(t, errs, 1)
		assert.ErrorContains(t, errs[0], "connection refused")
	})
}
