package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_JaegerCR_Gather(t *testing.T) {
	var jaegerYAML = `
apiVersion: jaegertracing.io/v1
kind: Jaeger
metadata:
    name: testing-jaeger
`
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		jaegerResource: "JaegersList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testJaegerCR := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(jaegerYAML), nil, testJaegerCR)
	assert.NoError(t, err, "unable to decode jaeger YAML")
	_, err = client.Resource(jaegerResource).Create(context.Background(), testJaegerCR, metav1.CreateOptions{})
	assert.NoError(t, err, "unable to create fake jaeger")

	records, errs := gatherJaegerCR(context.Background(), client)
	if assert.Empty(t, errs, "unexpected errors while gathering Jaeger CRs") {
		assert.Len(t, records, 1, "unexpected number or records")
		assert.Equal(t, "config/jaegertracing.io/testing-jaeger", records[0].Name)
	}
}
