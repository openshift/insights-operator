package clusterconfig

import (
	"context"
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

func newFakeInferenceServiceClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			inferenceServiceGVR: "InferenceServiceList",
		},
	)
}

func createInferenceService(t *testing.T, client dynamic.Interface, namespace, name string, fields map[string]interface{}) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.kserve.io/v1beta1",
			"kind":       "InferenceService",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"predictor": map[string]interface{}{
					"model": fields,
				},
			},
		},
	}
	_, err := client.Resource(inferenceServiceGVR).Namespace(namespace).Create(
		context.Background(), obj, metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("failed to create InferenceService: %v", err)
	}
}

func Test_gatherInferenceServices(t *testing.T) {
	t.Run("empty cluster returns no records", func(t *testing.T) {
		client := newFakeInferenceServiceClient()
		records, errs := gatherInferenceServices(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("collects selected fields from all namespaces", func(t *testing.T) {
		client := newFakeInferenceServiceClient()
		createInferenceService(t, client, "ns-a", "model-a", map[string]interface{}{
			"modelFormat": map[string]interface{}{
				"name":    "onnx",
				"version": "1",
			},
			"runtime":         "kserve-ovms",
			"protocolVersion": "v2",
			"storageUri":      "s3://my-bucket/models/model-a",
		})
		createInferenceService(t, client, "ns-b", "model-b", map[string]interface{}{
			"modelFormat": map[string]interface{}{
				"name": "tensorflow",
			},
		})

		records, errs := gatherInferenceServices(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)
		assert.Equal(t, "config/serving.kserve.io/inferenceservices", records[0].Name)

		services, ok := records[0].Item.(record.JSONMarshaller).Object.([]inferenceService)
		assert.True(t, ok)
		assert.Len(t, services, 2)

		// Verify model-a fields are collected
		var svcA inferenceService
		for _, s := range services {
			if s.Name == "model-a" {
				svcA = s
				break
			}
		}
		assert.Equal(t, "ns-a", svcA.Namespace)
		assert.Equal(t, "onnx", svcA.ModelFormatName)
		assert.Equal(t, "1", svcA.ModelFormatVer)
		assert.Equal(t, "kserve-ovms", svcA.Runtime)
		assert.Equal(t, "v2", svcA.ProtocolVersion)

		// Verify model-b partial fields
		var svcB inferenceService
		for _, s := range services {
			if s.Name == "model-b" {
				svcB = s
				break
			}
		}
		assert.Equal(t, "ns-b", svcB.Namespace)
		assert.Equal(t, "tensorflow", svcB.ModelFormatName)
		assert.Empty(t, svcB.ModelFormatVer)
		assert.Empty(t, svcB.Runtime)
		assert.Empty(t, svcB.ProtocolVersion)
	})

	t.Run("storageUri is not collected", func(t *testing.T) {
		client := newFakeInferenceServiceClient()
		createInferenceService(t, client, "ns-a", "model-a", map[string]interface{}{
			"modelFormat": map[string]interface{}{"name": "onnx"},
			"storageUri":  "s3://sensitive-bucket/path/to/model",
		})

		records, errs := gatherInferenceServices(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)

		services := records[0].Item.(record.JSONMarshaller).Object.([]inferenceService)
		assert.Len(t, services, 1)
		// No storageUri field on the struct — it's simply not present
		assert.Equal(t, "onnx", services[0].ModelFormatName)
	})
}
