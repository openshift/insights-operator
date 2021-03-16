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

func TestSAPDatahubs(t *testing.T) {
	// Initialize the fake dynamic client.
	var datahubYAML = `apiVersion: installers.datahub.sap.com/v1alpha1
kind: DataHub
metadata:
    name: example-datahub
    namespace: example-namespace
`

	datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}
	datahubsClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		datahubsResource: "DataHubsList",
	})

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testDatahub := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(datahubYAML), nil, testDatahub)
	if err != nil {
		t.Fatal("unable to decode datahub YAML", err)
	}

	records, errs := gatherSAPDatahubs(context.Background(), datahubsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 0 records because there is no datahubs resource in the namespace.
	if len(records) != 0 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}

	// Create the DataHubs resource and now the SCCs and CRBs should be gathered.
	datahubsClient.Resource(datahubsResource).Namespace("example-namespace").Create(context.Background(), testDatahub, metav1.CreateOptions{})

	records, errs = gatherSAPDatahubs(context.Background(), datahubsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 1 record because the pod is now in the same namespace as a datahubs resource.
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}
}
