package clusterconfig

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestOLMOperatorsGather(t *testing.T) {
	f, err := os.Open("testdata/olm_operator_1.yaml")
	if err != nil {
		t.Fatal("test failed to read OLM operator data", err)
	}
	olmOpContent, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal("error reading test data file", err)
	}
	gvr := schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1", Resource: "operators"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "OperatorsList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testOLMOperator := &unstructured.Unstructured{}

	_, _, err = decUnstructured.Decode(olmOpContent, nil, testOLMOperator)
	if err != nil {
		t.Fatal("unable to decode OLM operator ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testOLMOperator, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake OLM operator ", err)
	}

	ctx := context.Background()
	records, errs := gatherOLMOperators(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	ooa, ok := records[0].Item.(OlmOperatorAnonymizer)
	if !ok {
		t.Fatalf("returned item is not of type OlmOperatorAnonymizer")
	}
	if ooa.operators[0].Name != "test-olm-operator" {
		t.Fatalf("unexpected name of gathered OLM operator %s", ooa.operators[0])
	}
	if ooa.operators[0].Version != "v1.2.3" {
		t.Fatalf("unexpected version of gathered OLM operator %s", ooa.operators[0])
	}
}
