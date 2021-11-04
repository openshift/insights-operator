package clusterconfig

import (
	"context"
	"encoding/json"
	"strings"
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
	recordName := "config/routes/openshift-routes/some-route"
	testOpenshiftRouteResource := &unstructured.Unstructured{}

	_, _, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode([]byte(openshiftRouteYAML), nil, testOpenshiftRouteResource)
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
	if errs != nil && len(errs) > 0 {
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

	// ensure record only contains non-secret information
	ensureInformationDoesNotExist(t, item, "spec", "host")
	ensureInformationDoesNotExist(t, item, "spec", "tls")

}

func ensureInformationDoesNotExist(t *testing.T, item map[string]interface{}, fields ...string) {
	found, _, err := unstructured.NestedFieldNoCopy(item, fields...)
	if err != nil {
		t.Errorf("gatherOpenshiftRoutes() error while searching for NestedFieldNoCopy %s - %v", strings.Join(fields, "."), err)
	}
	if found != nil {
		t.Errorf("gatherOpenshiftRoutes() error found %s - %v", strings.Join(fields, "."), found)
	}
}
