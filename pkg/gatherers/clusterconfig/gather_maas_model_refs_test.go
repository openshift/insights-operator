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

func newFakeMaasModelRefClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			maasModelRefGVR: "MaasModelRefList",
		},
	)
}

func createMaasModelRef(t *testing.T, client dynamic.Interface, namespace, name, kind string) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "maas.opendatahub.io/v1alpha1",
			"kind":       "MaasModelRef",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"modelRef": map[string]interface{}{
					"kind": kind,
					"name": "referenced-model",
				},
			},
		},
	}
	_, err := client.Resource(maasModelRefGVR).Namespace(namespace).Create(
		context.Background(), obj, metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("failed to create MaasModelRef: %v", err)
	}
}

func Test_gatherMaasModelRefs(t *testing.T) {
	t.Run("empty cluster returns no records", func(t *testing.T) {
		client := newFakeMaasModelRefClient()
		records, errs := gatherMaasModelRefs(context.Background(), client)
		assert.Empty(t, errs)
		assert.Empty(t, records)
	})

	t.Run("collects kind from all namespaces", func(t *testing.T) {
		const nameA, nameB = "ref-a", "ref-b"
		client := newFakeMaasModelRefClient()
		createMaasModelRef(t, client, "ns-a", nameA, "ExternalModel")
		createMaasModelRef(t, client, "ns-b", nameB, "InferenceService")

		records, errs := gatherMaasModelRefs(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)
		assert.Equal(t, "config/maasmodelrefs", records[0].Name)

		refs, ok := records[0].Item.(record.JSONMarshaller).Object.([]maasModelRef)
		assert.True(t, ok)
		assert.Len(t, refs, 2)

		var refA, refB maasModelRef
		for _, r := range refs {
			switch r.Name {
			case nameA:
				refA = r
			case nameB:
				refB = r
			}
		}
		assert.Equal(t, "ns-a", refA.Namespace)
		assert.Equal(t, "ExternalModel", refA.Kind)
		assert.Equal(t, "ns-b", refB.Namespace)
		assert.Equal(t, "InferenceService", refB.Kind)
	})

	t.Run("excludes referenced model name even when set on source object", func(t *testing.T) {
		client := newFakeMaasModelRefClient()
		createMaasModelRef(t, client, "ns-c", "ref-c", "ExternalModel")

		records, errs := gatherMaasModelRefs(context.Background(), client)
		assert.Empty(t, errs)
		assert.Len(t, records, 1)

		refs, ok := records[0].Item.(record.JSONMarshaller).Object.([]maasModelRef)
		assert.True(t, ok)
		assert.Len(t, refs, 1)

		marshaled, err := json.Marshal(refs)
		assert.NoError(t, err)
		assert.NotContains(t, string(marshaled), "referenced-model")
	})
}
