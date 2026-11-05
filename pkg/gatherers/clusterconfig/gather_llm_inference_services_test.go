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

func newFakeLLMInferenceServiceClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			llmInferenceServiceGVR: "LLMInferenceServiceList",
		},
	)
}

func createLLMInferenceService(t *testing.T, client dynamic.Interface, namespace, name string, spec map[string]interface{}) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.kserve.io/v1alpha1",
			"kind":       "LLMInferenceService",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
	_, err := client.Resource(llmInferenceServiceGVR).Namespace(namespace).Create(
		context.Background(), obj, metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("failed to create LLMInferenceService: %v", err)
	}
}

func setLLMInferenceServiceStatus(t *testing.T, client dynamic.Interface, namespace, name string, status map[string]interface{}) {
	t.Helper()
	obj, err := client.Resource(llmInferenceServiceGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get LLMInferenceService: %v", err)
	}
	obj.Object["status"] = status
	_, err = client.Resource(llmInferenceServiceGVR).Namespace(namespace).UpdateStatus(context.Background(), obj, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update LLMInferenceService status: %v", err)
	}
}

// fullLLMInferenceServiceSpec mirrors a disaggregated-serving deployment with LoRA
// adapters, a custom scheduler config, scaling bounds, and tracing enabled.
func fullLLMInferenceServiceSpec() map[string]interface{} {
	return map[string]interface{}{
		"model": map[string]interface{}{
			"name": "meta-llama/Llama-3-70b-instruct",
			"lora": map[string]interface{}{
				"adapters": []interface{}{
					map[string]interface{}{"name": "finance-adapter", "uri": "s3://acme-ml/adapters/finance"},
					map[string]interface{}{"name": "support-adapter", "uri": "s3://acme-ml/adapters/support"},
				},
				"maxAdapters": int64(4),
			},
		},
		"replicas": int64(2),
		"prefill": map[string]interface{}{
			"replicas": int64(1),
		},
		"parallelism": map[string]interface{}{
			"tensor":   int64(4),
			"pipeline": int64(1),
			"data":     int64(2),
			"expert":   true,
		},
		"router": map[string]interface{}{
			"scheduler": map[string]interface{}{
				"config": map[string]interface{}{
					"inline": map[string]interface{}{
						"plugins":     []interface{}{"least-request"},
						"authToken":   "super-secret-token",
						"gatewayHost": "internal.example.com",
					},
				},
			},
		},
		"scaling": map[string]interface{}{
			"minReplicas": int64(1),
			"maxReplicas": int64(4),
		},
		"tracing": map[string]interface{}{
			"exporterEndpoint": "http://otel-collector:4317",
		},
		"template": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "main",
					"image": "registry.redhat.io/rhaii/vllm-cuda-rhel9:latest",
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu":            "4",
							"memory":         "16Gi",
							"nvidia.com/gpu": "1",
						},
					},
					"env": []interface{}{
						map[string]interface{}{"name": "HF_TOKEN", "value": "hf_supersecret"},
					},
				},
			},
		},
	}
}

func Test_gatherLLMInferenceServices(t *testing.T) {
	t.Run("empty cluster returns no records", func(t *testing.T) {
		client := newFakeLLMInferenceServiceClient()
		records, errs := gatherLLMInferenceServices(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("collects selected fields from all namespaces", func(t *testing.T) {
		client := newFakeLLMInferenceServiceClient()
		createLLMInferenceService(t, client, "ns-a", "llama-full", fullLLMInferenceServiceSpec())
		setLLMInferenceServiceStatus(t, client, "ns-a", "llama-full", map[string]interface{}{
			"workloads": map[string]interface{}{
				"scheduler": map[string]interface{}{"kind": "Deployment", "name": "llama-full-scheduler"},
			},
		})
		createLLMInferenceService(t, client, "ns-b", "minimal-llm", map[string]interface{}{
			"model": map[string]interface{}{"name": "meta-llama/Llama-3-8B"},
		})

		records, errs := gatherLLMInferenceServices(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 2)
		// one record per namespace, in a stable order
		assert.Equal(t, "config/serving.kserve.io/ns-a/llminferenceservices", records[0].Name)
		assert.Equal(t, "config/serving.kserve.io/ns-b/llminferenceservices", records[1].Name)

		services, ok := records[0].Item.(record.JSONMarshaller).Object.([]llmInferenceService)
		assert.True(t, ok)
		assert.Len(t, services, 1)

		full := findLLMInferenceService(services, "llama-full")
		assert.Equal(t, "ns-a", full.Namespace)
		assert.NotNil(t, full.Model)
		assert.Equal(t, "meta-llama/Llama-3-70b-instruct", full.Model.Name)
		assert.ElementsMatch(t, []string{"finance-adapter", "support-adapter"}, full.Model.LoraAdapterNames)
		assert.NotNil(t, full.Model.LoraMaxAdapters)
		assert.Equal(t, int64(4), *full.Model.LoraMaxAdapters)
		assert.NotNil(t, full.Replicas)
		assert.Equal(t, int64(2), *full.Replicas)
		assert.True(t, full.PrefillPresent)
		assert.NotNil(t, full.PrefillReplicas)
		assert.Equal(t, int64(1), *full.PrefillReplicas)
		assert.Equal(t, &llmParallelism{Tensor: 4, Pipeline: 1, Data: 2, Expert: boolPtr(true)}, full.Parallelism)
		assert.True(t, full.RouterSchedulerPresent)
		assert.Equal(t, []interface{}{"least-request"}, full.RouterSchedulerConfigInline["plugins"])
		assert.True(t, full.WorkloadsSchedulerPresent)
		assert.Equal(t, &llmScaling{MinReplicas: int64Ptr(1), MaxReplicas: int64Ptr(4)}, full.Scaling)
		assert.True(t, full.TracingPresent)
		assert.Len(t, full.Containers, 1)
		assert.Equal(t, "registry.redhat.io/rhaii/vllm-cuda-rhel9:latest", full.Containers[0].Image)
		assert.Equal(t, map[string]string{"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"}, full.Containers[0].Resources)

		nsBServices, ok := records[1].Item.(record.JSONMarshaller).Object.([]llmInferenceService)
		assert.True(t, ok)
		assert.Len(t, nsBServices, 1)

		minimal := findLLMInferenceService(nsBServices, "minimal-llm")
		assert.Equal(t, "ns-b", minimal.Namespace)
		assert.NotNil(t, minimal.Model)
		assert.Equal(t, "meta-llama/Llama-3-8B", minimal.Model.Name)
		// absent rather than false/zero
		assert.False(t, minimal.PrefillPresent)
		assert.Nil(t, minimal.PrefillReplicas)
		assert.Nil(t, minimal.Parallelism)
		assert.False(t, minimal.RouterSchedulerPresent)
		assert.Nil(t, minimal.RouterSchedulerConfigInline)
		assert.False(t, minimal.WorkloadsSchedulerPresent)
		assert.Nil(t, minimal.Scaling)
		assert.False(t, minimal.TracingPresent)
		assert.Empty(t, minimal.Containers)
	})

	t.Run("adapters uri and container env are never collected", func(t *testing.T) {
		client := newFakeLLMInferenceServiceClient()
		createLLMInferenceService(t, client, "ns-a", "llama-full", fullLLMInferenceServiceSpec())

		records, errs := gatherLLMInferenceServices(context.Background(), client)
		assert.Empty(t, errs)

		marshaled, err := records[0].Item.Marshal()
		assert.NoError(t, err)
		assert.NotContains(t, string(marshaled), "s3://acme-ml")
		assert.NotContains(t, string(marshaled), "hf_supersecret")
		assert.NotContains(t, string(marshaled), "HF_TOKEN")
	})

	t.Run("sensitive keys in the inline scheduler config are redacted", func(t *testing.T) {
		client := newFakeLLMInferenceServiceClient()
		createLLMInferenceService(t, client, "ns-a", "llama-full", fullLLMInferenceServiceSpec())

		records, errs := gatherLLMInferenceServices(context.Background(), client)
		assert.Empty(t, errs)

		services := records[0].Item.(record.JSONMarshaller).Object.([]llmInferenceService)
		assert.Len(t, services, 1)
		inline := services[0].RouterSchedulerConfigInline
		assert.NotEqual(t, "super-secret-token", inline["authToken"])
		assert.NotEqual(t, "internal.example.com", inline["gatewayHost"])
		assert.Equal(t, []interface{}{"least-request"}, inline["plugins"])
	})

	t.Run("limits the number of gathered services across all namespaces", func(t *testing.T) {
		client := newFakeLLMInferenceServiceClient()
		for i := 0; i < llmInferenceServiceRecordsLimit+5; i++ {
			namespace := fmt.Sprintf("ns-%d", i%2)
			createLLMInferenceService(t, client, namespace, fmt.Sprintf("llm-%03d", i), map[string]interface{}{})
		}

		records, errs := gatherLLMInferenceServices(context.Background(), client)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "limiting to 100")
		assert.Len(t, records, 2)

		var gathered int
		for _, r := range records {
			gathered += len(r.Item.(record.JSONMarshaller).Object.([]llmInferenceService))
		}
		assert.Equal(t, llmInferenceServiceRecordsLimit, gathered)
	})
}

func findLLMInferenceService(services []llmInferenceService, name string) llmInferenceService {
	for i := range services {
		if services[i].Name == name {
			return services[i]
		}
	}
	return llmInferenceService{}
}

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}
