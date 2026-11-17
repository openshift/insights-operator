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
					"name": "my-http-output",
					"type": "http",
					"http": map[string]interface{}{
						"url":      "https://my-secret-endpoint.example.com/logs",
						"proxyURL": "http://proxy.internal:8080",
						"headers": map[string]interface{}{
							"Authorization": "Bearer my-secret-token",
							"X-Custom":      "value",
						},
						"authentication": map[string]interface{}{
							"username": map[string]interface{}{
								"key":           "username",
								"secretName":    "my-auth-secret",
								"containerPath": "/var/run/ocp-collector/secrets/my-auth-secret",
							},
						},
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
					"name": "kafka-output",
					"type": "kafka",
					"kafka": map[string]interface{}{
						"url": "https://kafka.example.com:9093/topic",
						"brokers": []interface{}{
							map[string]interface{}{"url": "kafka-0.example.com:9093"},
							map[string]interface{}{"url": "kafka-1.example.com:9093"},
						},
						"topic": "logs-topic",
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
						"authentication": map[string]interface{}{
							"token": map[string]interface{}{
								"from": "serviceAccount",
							},
						},
					},
				},
				map[string]interface{}{
					"name": "cloudwatch-output",
					"type": "cloudwatch",
					"cloudwatch": map[string]interface{}{
						"url":       "https://logs.us-east-1.amazonaws.com",
						"groupName": "my-log-group",
						"authentication": map[string]interface{}{
							"type": "awsAccessKey",
							"awsAccessKey": map[string]interface{}{
								"keyId": map[string]interface{}{
									"key":        "aws_access_key_id",
									"secretName": "cw-secret",
								},
							},
						},
					},
				},
			},
		},
	}

	err := anonymizeClusterLogForwarderOutputs(obj)
	assert.NoError(t, err)

	outputs, _, _ := unstructured.NestedSlice(obj, "spec", "outputs")

	// HTTP output: tls, auth, url, headers, proxyURL, nested auth all removed
	httpOutput := outputs[0].(map[string]interface{})
	_, hasTLS := httpOutput["tls"]
	assert.False(t, hasTLS, "tls should be removed")
	_, hasAuth := httpOutput["authentication"]
	assert.False(t, hasAuth, "top-level authentication should be removed")
	httpSpec := httpOutput["http"].(map[string]interface{})
	_, hasURL := httpSpec["url"]
	assert.False(t, hasURL, "http url should be removed")
	_, hasHeaders := httpSpec["headers"]
	assert.False(t, hasHeaders, "http headers should be removed")
	_, hasProxy := httpSpec["proxyURL"]
	assert.False(t, hasProxy, "http proxyURL should be removed")
	_, hasNestedAuth := httpSpec["authentication"]
	assert.False(t, hasNestedAuth, "http nested authentication should be removed")

	// Kafka output: url and brokers removed, topic preserved
	kafkaOutput := outputs[1].(map[string]interface{})
	kafkaSpec := kafkaOutput["kafka"].(map[string]interface{})
	_, hasKafkaURL := kafkaSpec["url"]
	assert.False(t, hasKafkaURL, "kafka url should be removed")
	_, hasBrokers := kafkaSpec["brokers"]
	assert.False(t, hasBrokers, "kafka brokers should be removed")
	assert.Equal(t, "logs-topic", kafkaSpec["topic"], "kafka topic should be preserved")

	// LokiStack output: authentication removed, target preserved
	lokiOutput := outputs[2].(map[string]interface{})
	lokiSpec := lokiOutput["lokiStack"].(map[string]interface{})
	target := lokiSpec["target"].(map[string]interface{})
	assert.Equal(t, "my-lokistack", target["name"], "lokiStack target name should be preserved")
	assert.Equal(t, "openshift-logging", target["namespace"], "lokiStack target namespace should be preserved")
	_, hasLokiAuth := lokiSpec["authentication"]
	assert.False(t, hasLokiAuth, "lokiStack authentication should be removed")

	// CloudWatch output: url and authentication removed, groupName preserved
	cwOutput := outputs[3].(map[string]interface{})
	cwSpec := cwOutput["cloudwatch"].(map[string]interface{})
	_, hasCWURL := cwSpec["url"]
	assert.False(t, hasCWURL, "cloudwatch url should be removed")
	_, hasCWAuth := cwSpec["authentication"]
	assert.False(t, hasCWAuth, "cloudwatch authentication should be removed")
	assert.Equal(t, "my-log-group", cwSpec["groupName"], "cloudwatch groupName should be preserved")
}
