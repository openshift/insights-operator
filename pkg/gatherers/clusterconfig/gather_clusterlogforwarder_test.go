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

func TestGatherClusterLogForwarder(t *testing.T) {
	var clfYAMLTmpl = `
apiVersion: observability.openshift.io/v1
kind: ClusterLogForwarder
metadata:
    name: test-clf-%d
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
			resourcesNumber:         clusterLogForwarderResourceLimit,
			expectedErrors:          nil,
			expectedNumberOfRecords: clusterLogForwarderResourceLimit,
		},
		{
			name:            "too many resources in right namespace",
			namespace:       "openshift-logging",
			resourcesNumber: clusterLogForwarderResourceLimit + 1,
			expectedErrors: []error{
				fmt.Errorf("found %d resources, limit (%d) reached", clusterLogForwarderResourceLimit+1, clusterLogForwarderResourceLimit),
			},
			expectedNumberOfRecords: clusterLogForwarderResourceLimit,
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
			clusterLogForwarderResource: "ClusterLogForwardersList",
		})
		decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		testCLF := &unstructured.Unstructured{}

		for idx := 0; idx < tt.resourcesNumber; idx++ {
			clfYAML := fmt.Sprintf(clfYAMLTmpl, idx, tt.namespace)
			_, _, err := decUnstructured.Decode([]byte(clfYAML), nil, testCLF)
			if err != nil {
				t.Fatal("unable to decode clusterlogforwarder ", err)
			}
			_, err = client.Resource(clusterLogForwarderResource).
				Namespace(tt.namespace).
				Create(context.Background(), testCLF, metav1.CreateOptions{})
			if err != nil {
				t.Fatal("unable to create fake clusterlogforwarder ", err)
			}
		}

		ctx := context.Background()
		records, errs := gatherClusterLogForwarder(ctx, client)

		assert.Equal(t, tt.expectedNumberOfRecords, len(records))
		assert.Equal(t, tt.expectedErrors, errs)
	}
}

func TestAnonymizeClusterLogForwarderOutputs(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"outputs": []interface{}{
				map[string]interface{}{
					"name": "my-output",
					"type": "http",
					"http": map[string]interface{}{
						"url": "https://my-secret-endpoint.example.com/logs",
					},
					"tls": map[string]interface{}{
						"certificate": map[string]interface{}{
							"key":  "tls.crt",
							"name": "my-tls-secret",
						},
					},
					"authentication": map[string]interface{}{
						"token": map[string]interface{}{
							"from": "secret",
							"secret": map[string]interface{}{
								"name": "my-auth-secret",
								"key":  "token",
							},
						},
					},
				},
				map[string]interface{}{
					"name": "loki-output",
					"type": "lokiStack",
					"lokiStack": map[string]interface{}{
						"target": map[string]interface{}{
							"name":      "my-lokistack",
							"namespace": "openshift-logging",
						},
					},
				},
			},
		},
	}

	err := anonymizeClusterLogForwarderOutputs(obj)
	assert.NoError(t, err)

	outputs, _, _ := unstructured.NestedSlice(obj, "spec", "outputs")

	httpOutput := outputs[0].(map[string]interface{})
	_, hasTLS := httpOutput["tls"]
	assert.False(t, hasTLS, "tls should be removed")
	_, hasAuth := httpOutput["authentication"]
	assert.False(t, hasAuth, "authentication should be removed")
	httpSpec := httpOutput["http"].(map[string]interface{})
	_, hasURL := httpSpec["url"]
	assert.False(t, hasURL, "url should be removed")

	lokiOutput := outputs[1].(map[string]interface{})
	lokiSpec := lokiOutput["lokiStack"].(map[string]interface{})
	target := lokiSpec["target"].(map[string]interface{})
	assert.Equal(t, "my-lokistack", target["name"], "lokiStack target should be preserved")
}
