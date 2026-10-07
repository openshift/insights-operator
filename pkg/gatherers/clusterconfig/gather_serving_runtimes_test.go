package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/openshift/insights-operator/pkg/record"
)

func newFakeServingRuntimeClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			servingRuntimeGVR:        "ServingRuntimeList",
			clusterServingRuntimeGVR: "ClusterServingRuntimeList",
		},
	)
}

func createServingRuntime(t *testing.T, client dynamic.Interface, namespace, name string, spec map[string]interface{}) {
	t.Helper()
	metadata := map[string]interface{}{"name": name}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.kserve.io/v1alpha1",
			"kind":       "ServingRuntime",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
	_, err := client.Resource(servingRuntimeGVR).Namespace(namespace).Create(
		context.Background(), obj, metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("failed to create ServingRuntime: %v", err)
	}
}

// vllmRuntimeSpec mirrors the real RHOAI vllm-multinode-runtime template.
func vllmRuntimeSpec() map[string]interface{} {
	return map[string]interface{}{
		"supportedModelFormats": []interface{}{
			map[string]interface{}{"name": "vLLM", "autoSelect": true},
		},
		"multiModel": false,
		"containers": []interface{}{
			map[string]interface{}{
				"name":  "kserve-container",
				"image": "registry.redhat.io/rhaii/vllm-cuda-rhel9:latest",
				"args": []interface{}{
					"/mnt/models",
					"--port=8080",
					"--max-model-len=4096",
				},
				"env": []interface{}{
					map[string]interface{}{"name": "HF_TOKEN", "value": "hf_supersecret"},
				},
			},
		},
		"workerSpec": map[string]interface{}{
			"tensorParallelSize":   int64(1),
			"pipelineParallelSize": int64(2),
		},
	}
}

func Test_gatherServingRuntimes(t *testing.T) {
	t.Run("empty cluster returns no records", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		records, errs := gatherServingRuntimes(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("collects selected fields from all namespaces", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		createServingRuntime(t, client, "ns-a", "vllm-runtime", vllmRuntimeSpec())
		createServingRuntime(t, client, "ns-b", "ovms-runtime", map[string]interface{}{
			"supportedModelFormats": []interface{}{
				map[string]interface{}{"name": "onnx", "version": "1"},
				map[string]interface{}{"name": "tensorflow", "version": "2"},
			},
			"protocolVersions": []interface{}{"v2", "grpc-v2"},
		})

		records, errs := gatherServingRuntimes(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)
		assert.Equal(t, "config/serving.kserve.io/servingruntimes", records[0].Name)

		runtimes, ok := records[0].Item.(record.JSONMarshaller).Object.([]servingRuntime)
		assert.True(t, ok)
		assert.Len(t, runtimes, 2)

		vllm := findServingRuntime(runtimes, "vllm-runtime")
		assert.Equal(t, "ns-a", vllm.Namespace)
		assert.Equal(t, []servingRuntimeFormat{{Name: "vLLM"}}, vllm.SupportedModelFormats)
		assert.NotNil(t, vllm.MultiModel)
		assert.False(t, *vllm.MultiModel)
		assert.Empty(t, vllm.ProtocolVersions)
		assert.Equal(t, &servingRuntimeWorkerSpec{TensorParallelSize: 1, PipelineParallelSize: 2}, vllm.WorkerSpec)
		assert.Len(t, vllm.Containers, 1)
		assert.Equal(t, "kserve-container", vllm.Containers[0].Name)
		assert.Equal(t, "registry.redhat.io/rhaii/vllm-cuda-rhel9:latest", vllm.Containers[0].Image)

		ovms := findServingRuntime(runtimes, "ovms-runtime")
		assert.Equal(t, "ns-b", ovms.Namespace)
		assert.Equal(t, []string{"v2", "grpc-v2"}, ovms.ProtocolVersions)
		assert.Equal(t, []servingRuntimeFormat{
			{Name: "onnx", Version: "1"},
			{Name: "tensorflow", Version: "2"},
		}, ovms.SupportedModelFormats)
		// absent rather than false/zero
		assert.Nil(t, ovms.MultiModel)
		assert.Nil(t, ovms.WorkerSpec)
		assert.Empty(t, ovms.Containers)
	})

	t.Run("container env is never collected", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		createServingRuntime(t, client, "ns-a", "vllm-runtime", vllmRuntimeSpec())

		records, errs := gatherServingRuntimes(context.Background(), client)
		assert.Empty(t, errs)

		// The struct has no Env field at all, so the secret cannot reach the archive.
		marshaled, err := records[0].Item.Marshal()
		assert.NoError(t, err)
		assert.NotContains(t, string(marshaled), "hf_supersecret")
		assert.NotContains(t, string(marshaled), "HF_TOKEN")
	})

	t.Run("limits the number of gathered runtimes", func(t *testing.T) {
		client := newFakeServingRuntimeClient()
		for i := 0; i < servingRuntimeRecordsLimit+5; i++ {
			createServingRuntime(t, client, "ns-a", fmt.Sprintf("runtime-%03d", i), map[string]interface{}{})
		}

		records, errs := gatherServingRuntimes(context.Background(), client)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "limiting to 100")
		assert.Len(t, records, 1)

		runtimes := records[0].Item.(record.JSONMarshaller).Object.([]servingRuntime)
		assert.Len(t, runtimes, servingRuntimeRecordsLimit)
	})
}

func Test_anonymizeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "benign tuning flags are kept verbatim",
			args:     []string{"--port=8080", "--model=/mnt/models", "--max-model-len=4096", "--dtype=float16"},
			expected: []string{"--port=8080", "--model=/mnt/models", "--max-model-len=4096", "--dtype=float16"},
		},
		{
			name:     "bare args without a value are kept",
			args:     []string{"/mnt/models", "--headless"},
			expected: []string{"/mnt/models", "--headless"},
		},
		{
			name:     "sensitive flag names have their value obfuscated",
			args:     []string{"--hf-token=hf_abc123", "--api-key=sk-secret", "--auth-mode=bearer"},
			expected: []string{"--hf-token=xxxxxxxxx", "--api-key=xxxxxxxxx", "--auth-mode=xxxxxx"},
		},
		{
			name:     "URI-like values are obfuscated even without a sensitive flag name",
			args:     []string{"--download-dir=s3://acme-ml/models", "--peer=user:pw@host"},
			expected: []string{"--download-dir=xxxxxxxxxxxxxxxxxxx", "--peer=xxxxxxxxxxxx"},
		},
		{
			name:     "bare URI-like arg is obfuscated",
			args:     []string{"s3://acme-ml/models"},
			expected: []string{"xxxxxxxxxxxxxxxxxxx"},
		},
		{
			name:     "env var indirections are kept verbatim",
			args:     []string{"--master-addr=$(POD_IP)", "--served-model-name={{.Name}}"},
			expected: []string{"--master-addr=$(POD_IP)", "--served-model-name={{.Name}}"},
		},
		{
			name: "accepted false positive: tokenizer flags match the shared sensitive pattern",
			args: []string{"--tokenizer-mode=auto"},
			// sensitiveFieldPattern is deliberately blunt and matches "token" inside
			// "tokenizer". Pinned here so the behavior is a decision, not a surprise.
			expected: []string{"--tokenizer-mode=xxxx"},
		},
		{
			name:     "only the first equals sign splits the arg",
			args:     []string{"--extra=a=b"},
			expected: []string{"--extra=a=b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, anonymizeArgs(tt.args))
		})
	}
}

func findServingRuntime(runtimes []servingRuntime, name string) servingRuntime {
	for _, r := range runtimes {
		if r.Name == name {
			return r
		}
	}
	return servingRuntime{}
}
