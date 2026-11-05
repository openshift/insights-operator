package clusterconfig

import (
	"context"
	"encoding/json"
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

func newFakeExternalModelClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			externalModelGVR: "ExternalModelList",
		},
	)
}

func createExternalModel(t *testing.T, client dynamic.Interface, namespace, name, provider, targetModel string) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "maas.opendatahub.io/v1alpha1",
			"kind":       "ExternalModel",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"provider":    provider,
				"targetModel": targetModel,
				"endpoint":    "https://api.example.com/v1",
				"credentialRef": map[string]interface{}{
					"name": "openai-api-key",
				},
			},
		},
	}
	_, err := client.Resource(externalModelGVR).Namespace(namespace).Create(
		context.Background(), obj, metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("failed to create ExternalModel: %v", err)
	}
}

func Test_gatherExternalModels(t *testing.T) {
	t.Run("empty cluster returns no records", func(t *testing.T) {
		client := newFakeExternalModelClient()
		records, errs := gatherExternalModels(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("collects provider and targetModel from all namespaces", func(t *testing.T) {
		const nameA, nameB = "model-a", "model-b"
		client := newFakeExternalModelClient()
		createExternalModel(t, client, "ns-a", nameA, "openai", "gpt-4o")
		createExternalModel(t, client, "ns-b", nameB, "anthropic", "claude-sonnet")

		records, errs := gatherExternalModels(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)
		assert.Equal(t, "config/externalmodels", records[0].Name)

		models, ok := records[0].Item.(record.JSONMarshaller).Object.([]externalModel)
		assert.True(t, ok)
		assert.Len(t, models, 2)

		var modelA, modelB externalModel
		for _, m := range models {
			switch m.Name {
			case nameA:
				modelA = m
			case nameB:
				modelB = m
			}
		}
		assert.Equal(t, "ns-a", modelA.Namespace)
		assert.Equal(t, "openai", modelA.Provider)
		assert.Equal(t, "gpt-4o", modelA.TargetModel)
		assert.Equal(t, "ns-b", modelB.Namespace)
		assert.Equal(t, "anthropic", modelB.Provider)
		assert.Equal(t, "claude-sonnet", modelB.TargetModel)
	})

	t.Run("excludes endpoint and credentialRef fields even when set on source object", func(t *testing.T) {
		client := newFakeExternalModelClient()
		createExternalModel(t, client, "ns-c", "model-c", "openai", "gpt-4o")

		records, errs := gatherExternalModels(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)

		models, ok := records[0].Item.(record.JSONMarshaller).Object.([]externalModel)
		assert.True(t, ok)
		assert.Len(t, models, 1)

		marshaled, err := json.Marshal(models)
		assert.NoError(t, err)
		assert.NotContains(t, string(marshaled), "endpoint")
		assert.NotContains(t, string(marshaled), "credentialRef")
		assert.NotContains(t, string(marshaled), "api.example.com")
		assert.NotContains(t, string(marshaled), "openai-api-key")
	})
}
