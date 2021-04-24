package clusterconfig

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_PNCC(t *testing.T) {
	var pnccYAML = `apiVersion: controlplane.operator.openshift.io/v1alpha1
kind: PodNetworkConnectivityCheck
metadata:
    name: example-pncc
    namespace: example-namespace
status:
    failures:
      - success: false
        reason: TestReason
        message: TestMessage
`

	pnccClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		pnccGroupVersionResource: "PodNetworkConnectivityChecksList",
	})

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testPNCC := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(pnccYAML), nil, testPNCC)
	if err != nil {
		t.Fatal("unable to decode PNCC YAML", err)
	}

	// Check before creating the PNCC.
	records, errs := gatherPNCC(context.Background(), pnccClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors in the first run: %#v", errs)
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}
	rec := records[0]
	if rec.Name != "config/podnetworkconnectivitychecks" {
		t.Fatalf("unexpected name of record in the first run: %q", rec.Name)
	}
	recItem, ok := rec.Item.(record.JSONMarshaller)
	if !ok {
		t.Fatalf("unexpected type of record item in the first run: %q", rec.Name)
	}
	if !reflect.DeepEqual(recItem.Object, map[string]map[string]time.Time{}) {
		t.Fatalf("unexpected value of record item in the first run: %#v", recItem)
	}

	// Create the PNCC resource.
	_, _ = pnccClient.Resource(pnccGroupVersionResource).Namespace("example-namespace").Create(context.Background(), testPNCC, metav1.CreateOptions{})

	// Check after creating the PNCC.
	records, errs = gatherPNCC(context.Background(), pnccClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors in the second run: %#v", errs)
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}
	rec = records[0]
	if rec.Name != "config/podnetworkconnectivitychecks" {
		t.Fatalf("unexpected name of record in the second run: %q", rec.Name)
	}
	recItem, ok = rec.Item.(record.JSONMarshaller)
	if !ok {
		t.Fatalf("unexpected type of record item in the second run: %q", rec.Name)
	}
	if !reflect.DeepEqual(recItem.Object, map[string]map[string]time.Time{"TestReason": {"TestMessage": time.Time{}}}) {
		t.Fatalf("unexpected value of record item in the second run: %#v", recItem)
	}
}
