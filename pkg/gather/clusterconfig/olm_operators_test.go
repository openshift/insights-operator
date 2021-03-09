package clusterconfig

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestOLMOperatorsGather(t *testing.T) {
	olmOpContent, err := readFromFile("testdata/olm_operator_1.yaml")
	if err != nil {
		t.Fatal("test failed to read OLM operator data", err)
	}

	csvContent, err := readFromFile("testdata/csv_1.yaml")
	if err != nil {
		t.Fatal("test failed to read CSV ", err)
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	err = createUnstructuredResource(olmOpContent, client, operatorGVR)
	if err != nil {
		t.Fatal("cannot create OLM operator ", err)
	}
	err = createUnstructuredResource(csvContent, client, clusterServiceVersionGVR)
	if err != nil {
		t.Fatal("cannot create ClusterServiceVersion ", err)
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
	ooa, ok := records[0].Item.(record.JSONMarshaller).Object.([]olmOperator)
	if !ok {
		t.Fatalf("returned item is not of type []olmOperator")
	}
	if ooa[0].Name != "test-olm-operator" {
		t.Fatalf("unexpected name of gathered OLM operator %s", ooa[0].Name)
	}
	if ooa[0].Version != "v1.2.3" {
		t.Fatalf("unexpected version of gathered OLM operator %s", ooa[0].Version)
	}
	if len(ooa[0].Conditions) != 2 {
		t.Fatalf("unexpected number of conditions %s", ooa[0].Conditions...)
	}
}

func createUnstructuredResource(content []byte, client *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource) error {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	unstructuredResource := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode(content, nil, unstructuredResource)
	if err != nil {
		return err
	}

	_, err = client.Resource(gvr).Namespace("test-olm-operator").Create(context.Background(), unstructuredResource, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func readFromFile(filePath string) ([]byte, error) {
	f, err := os.Open(filePath)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return content, nil
}
