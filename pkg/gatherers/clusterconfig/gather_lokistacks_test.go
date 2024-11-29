package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestGatherLokiStacks(t *testing.T) {
	var lokiStackYAMLTmpl = `
apiVersion: loki.grafana.com/v1
kind: LokiStack
metadata:
    name: test-lokistack-%d
    namespace: %s
`

	tests := []struct {
		name                    string
		namespace               string
		resourcesNumber         int
		expectedErrors          []error
		expectedNumberOfRecords int
	}{
		{
			name:                    "one resource scenario",
			namespace:               "openshift-logging",
			resourcesNumber:         1,
			expectedErrors:          nil,
			expectedNumberOfRecords: 1,
		},
		{
			name:                    "several resources in right namespace",
			namespace:               "openshift-logging",
			resourcesNumber:         lokiStackResourceLimit,
			expectedErrors:          nil,
			expectedNumberOfRecords: lokiStackResourceLimit,
		},
		{
			name:            "too many resources in right namespace",
			namespace:       "openshift-logging",
			resourcesNumber: lokiStackResourceLimit + 1,
			expectedErrors: []error{
				fmt.Errorf("found %d resources, limit (%d) reached", lokiStackResourceLimit+1, lokiStackResourceLimit),
			},
			expectedNumberOfRecords: lokiStackResourceLimit,
		},
		{
			name:            "bad namespace",
			namespace:       "other-namespace",
			resourcesNumber: 1,
			expectedErrors: []error{
				fmt.Errorf("found resource in an unexpected namespace"),
			},
			expectedNumberOfRecords: 0,
		},
	}

	for _, tt := range tests {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			lokiStackResource: "LokiStacksList",
		})
		decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		testLokiStack := &unstructured.Unstructured{}

		for idx := 0; idx < tt.resourcesNumber; idx++ {
			lokiStackYAML := fmt.Sprintf(lokiStackYAMLTmpl, idx, tt.namespace)
			_, _, err := decUnstructured.Decode([]byte(lokiStackYAML), nil, testLokiStack)
			if err != nil {
				t.Fatal("unable to decode lokistack ", err)
			}
			_, err = client.Resource(lokiStackResource).
				Namespace(tt.namespace).
				Create(context.Background(), testLokiStack, metav1.CreateOptions{})
			if err != nil {
				t.Fatal("unable to create fake lokistack ", err)
			}
		}

		ctx := context.Background()
		records, errs := gatherLokiStack(ctx, client)

		assert.Equal(t, tt.expectedNumberOfRecords, len(records))
		assert.Equal(t, tt.expectedErrors, errs)
	}
}
