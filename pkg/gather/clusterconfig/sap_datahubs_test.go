package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_SAPDatahubs(t *testing.T) {
	// Initialize the fake dynamic client.
	datahubYAML1 := `apiVersion: installers.datahub.sap.com/v1alpha1
kind: DataHub
metadata:
    name: example-datahub
    namespace: example-namespace1
`
	datahubYAML2 := `apiVersion: installers.datahub.sap.com/v1alpha1
kind: DataHub
metadata:
    name: example-datahub
    namespace: example-namespace2
`

	datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}
	datahubsClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		datahubsResource: "DataHubsList",
	})

	decUnstructured1 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testDatahub1 := &unstructured.Unstructured{}
	_, _, err := decUnstructured1.Decode([]byte(datahubYAML1), nil, testDatahub1)
	if err != nil {
		t.Fatal("unable to decode datahub YAML", err)
	}

	decUnstructured2 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testDatahub2 := &unstructured.Unstructured{}
	_, _, err = decUnstructured2.Decode([]byte(datahubYAML2), nil, testDatahub2)
	if err != nil {
		t.Fatal("unable to decode datahub YAML", err)
	}

	records, errs := gatherSAPDatahubs(context.Background(), datahubsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 0 records because there is no datahubs resource yet.
	if len(records) != 0 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}

	// Create first datahubs resource.
	_, _ = datahubsClient.
		Resource(datahubsResource).
		Namespace("example-namespace1").
		Create(context.Background(), testDatahub1, metav1.CreateOptions{})

	records, errs = gatherSAPDatahubs(context.Background(), datahubsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 1 record because there is now one datahubs resource.
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}

	// Create second datahubs resource.
	_, _ = datahubsClient.
		Resource(datahubsResource).
		Namespace("example-namespace2").
		Create(context.Background(), testDatahub2, metav1.CreateOptions{})

	records, errs = gatherSAPDatahubs(context.Background(), datahubsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 2 record because there are now two datahubs resources.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}
}
