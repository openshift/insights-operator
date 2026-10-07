package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// createClusterServingRuntime creates a cluster-scoped object. The fake dynamic client
// infers scope from metadata.namespace, so it must not be set here.
func createClusterServingRuntime(t *testing.T, client dynamic.Interface, name string, spec map[string]interface{}) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.kserve.io/v1alpha1",
			"kind":       "ClusterServingRuntime",
			"metadata":   map[string]interface{}{"name": name},
			"spec":       spec,
		},
	}
	_, err := client.Resource(clusterServingRuntimeGVR).Create(context.Background(), obj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create ClusterServingRuntime: %v", err)
	}
}

func Test_gatherClusterServingRuntimes(t *testing.T) {
	t.Run("empty cluster returns no records", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		records, errs := gatherClusterServingRuntimes(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("collects selected fields and omits namespace", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		createClusterServingRuntime(t, client, "vllm-cuda-runtime", vllmRuntimeSpec())

		records, errs := gatherClusterServingRuntimes(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)
		assert.Equal(t, "config/serving.kserve.io/clusterservingruntimes", records[0].Name)

		runtimes, ok := records[0].Item.(record.JSONMarshaller).Object.([]servingRuntime)
		assert.True(t, ok)
		assert.Len(t, runtimes, 1)

		csr := runtimes[0]
		assert.Equal(t, "vllm-cuda-runtime", csr.Name)
		assert.Empty(t, csr.Namespace, "ClusterServingRuntime is cluster-scoped")
		assert.Equal(t, []servingRuntimeFormat{{Name: "vLLM"}}, csr.SupportedModelFormats)
		assert.Equal(t, &servingRuntimeWorkerSpec{TensorParallelSize: 1, PipelineParallelSize: 2}, csr.WorkerSpec)
		assert.Len(t, csr.Containers, 1)

		// namespace key must be omitted entirely from the serialized output
		marshaled, err := records[0].Item.Marshal()
		assert.NoError(t, err)
		assert.NotContains(t, string(marshaled), "namespace")
	})

	t.Run("does not pick up namespaced ServingRuntimes", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		createServingRuntime(t, client, "ns-a", "namespaced-runtime", vllmRuntimeSpec())

		records, errs := gatherClusterServingRuntimes(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("limits the number of gathered runtimes", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		for i := 0; i < servingRuntimeRecordsLimit+5; i++ {
			createClusterServingRuntime(t, client, fmt.Sprintf("runtime-%03d", i), map[string]interface{}{})
		}

		records, errs := gatherClusterServingRuntimes(context.Background(), client)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "limiting to 100")

		runtimes := records[0].Item.(record.JSONMarshaller).Object.([]servingRuntime)
		assert.Len(t, runtimes, servingRuntimeRecordsLimit)
	})
}
