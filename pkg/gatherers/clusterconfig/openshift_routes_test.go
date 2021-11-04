package clusterconfig

import (
	"context"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_GatherOpenshiftRoutes(t *testing.T) {
	var openshiftRouteYAML = `
apiVersion: route.openshift.io/v1
kind: Route
metadata:
    name: some-route
    namespace: openshift-routes
spec:
  host: www.example.com
  to:
    kind: Service
    name: frontend
  tls:
    certificate: |
      -----BEGIN CERTIFICATE-----
      [...]
      -----END CERTIFICATE-----
    insecureEdgeTerminationPolicy: Redirect
    key: |
      -----BEGIN RSA PRIVATE KEY-----
      [...]
      -----END RSA PRIVATE KEY-----
    termination: reencrypt
`
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		openshiftRouteResource: "RouteList",
	})
	totalRecords := 1
	recordName := "config/routes"
	testOpenshiftRouteResource := &unstructured.Unstructured{}

	_, _, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).
		Decode([]byte(openshiftRouteYAML), nil, testOpenshiftRouteResource)
	if err != nil {
		t.Fatal("unable to decode route ", err)
	}
	_, err = dynamicClient.
		Resource(openshiftRouteResource).
		Namespace("openshift-routes").
		Create(context.Background(), testOpenshiftRouteResource, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create fake resource %s", err)
	}

	records, errs := gatherOpenshiftRoutes(context.Background(), dynamicClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs[0].Error())
	}

	if len(records) != totalRecords {
		t.Errorf("gatherOpenshiftRoutes() got = %v, want %v", len(records), totalRecords)
	}

	if records[0].Name != recordName {
		t.Errorf("gatherOpenshiftRoutes() name = %v, want %v ", records[0].Name, recordName)
	}

	var item map[string]interface{}
	bytes, err := records[0].Item.Marshal(context.Background())
	if err != nil {
		t.Errorf("gatherOpenshiftRoutes() can't marshal record %v", err)
	}

	err = json.Unmarshal(bytes, &item)
	if err != nil {
		t.Errorf("gatherOpenshiftRoutes() can't unmarshal record %v", err)
	}

	if val, ok := item["count"]; ok {
		if int(val.(float64)) != 1 {
			t.Errorf("gatherOpenshiftRoutes() count must be 1, but was: %v", item["count"])
		}
	} else {
		t.Errorf("gatherOpenshiftRoutes() must contain a count entry %v", item)
	}
}
