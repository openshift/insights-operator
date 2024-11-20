package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_LokiStacks_Gather(t *testing.T) {
	var lokiStackYAML = `
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
    name: test-lokistack
    namespace: openshift-logging
`

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		lokiStackResource: "LokiStacksList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testLokiStack := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(lokiStackYAML), nil, testLokiStack)
	if err != nil {
		t.Fatal("unable to decode lokistack ", err)
	}
	_, err = client.Resource(lokiStackResource).
		Namespace("openshift-logging").
		Create(context.Background(), testLokiStack, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake lokistack ", err)
	}

	ctx := context.Background()
	records, errs := gatherLokiStack(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "namespace/openshift-logging/loki.grafana.com/lokistacks/test-lokistack" {
		t.Fatalf("unexpected lokistack name %s", records[0].Name)
	}
}

func Test_LokiStacks_Gather_SeveralResourcesRightNamespace(t *testing.T) {
	var lokiStackYAMLTmpl = `
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
    name: test-lokistack-%d
    namespace: openshift-logging
`

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		lokiStackResource: "LokiStacksList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testLokiStack := &unstructured.Unstructured{}

	for idx := range lokiStackResourceLimit {
		lokiStackYAML := fmt.Sprintf(lokiStackYAMLTmpl, idx)
		_, _, err := decUnstructured.Decode([]byte(lokiStackYAML), nil, testLokiStack)
		if err != nil {
			t.Fatal("unable to decode lokistack ", err)
		}
		_, err = client.Resource(lokiStackResource).
			Namespace("openshift-logging").
			Create(context.Background(), testLokiStack, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake lokistack ", err)
		}
	}

	ctx := context.Background()
	records, errs := gatherLokiStack(ctx, client)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != lokiStackResourceLimit {
		t.Fatalf("unexpected number or records %d", len(records))
	}
}

func Test_LokiStacks_Gather_TooManyResources(t *testing.T) {
	var lokiStackYAMLTmpl = `
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
    name: test-lokistack-%d
    namespace: openshift-logging
`

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		lokiStackResource: "LokiStacksList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testLokiStack := &unstructured.Unstructured{}

	// prepare limit + 1 resources in the right namespace
	for idx := range lokiStackResourceLimit + 1 {
		lokiStackYAML := fmt.Sprintf(lokiStackYAMLTmpl, idx)
		_, _, err := decUnstructured.Decode([]byte(lokiStackYAML), nil, testLokiStack)
		if err != nil {
			t.Fatal("unable to decode lokistack ", err)
		}
		_, err = client.Resource(lokiStackResource).
			Namespace("openshift-logging").
			Create(context.Background(), testLokiStack, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake lokistack ", err)
		}
	}

	ctx := context.Background()
	records, errs := gatherLokiStack(ctx, client)

	// ensuring an error was received
	if len(errs) != 1 {
		t.Errorf("1 error was expected, %d received", len(errs))
		return
	}

	if errs[0].Error() != fmt.Sprintf(
		"found %d resources, limit (%d) reached", lokiStackResourceLimit+1, lokiStackResourceLimit,
	) {
		t.Fatalf("unexpected error recorded: %#v", errs[0])
	}

	// only expect "limit" number of records
	if len(records) != lokiStackResourceLimit {
		t.Fatalf("unexpected number or records %d", len(records))
	}
}

func Test_LokiStacks_Gather_OtherNamespaces(t *testing.T) {
	var lokiStackYAMLTmpl = `
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
    name: test-lokistack-%d
    namespace: %s
`

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		lokiStackResource: "LokiStacksList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testLokiStack := &unstructured.Unstructured{}

	for idx, namespace := range []string{"openshift-logging", "other-namespace"} {
		lokiStackYAML := fmt.Sprintf(lokiStackYAMLTmpl, idx, namespace)
		_, _, err := decUnstructured.Decode([]byte(lokiStackYAML), nil, testLokiStack)
		if err != nil {
			t.Fatal("unable to decode lokistack ", err)
		}
		_, err = client.Resource(lokiStackResource).
			Namespace(namespace).
			Create(context.Background(), testLokiStack, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake lokistack ", err)
		}
	}

	ctx := context.Background()
	records, errs := gatherLokiStack(ctx, client)

	// ensuring an error was received
	if len(errs) != 1 {
		t.Errorf("1 error was expected, %d received", len(errs))
		return
	}

	if errs[0].Error() != "found resource in an unexpected namespace" {
		t.Fatalf("unexpected error recorded: %#v", errs[0])
	}

	// only expect "limit" number of records
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
}
