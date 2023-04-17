package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherTempoStackCR(t *testing.T) {
	var dataYAML = `
apiVersion: tempo.grafana.com/v1alpha1
kind: TempoStack
metadata:
  name: simplest
  namespace: openshift-operators
spec:
  storage:
    secret:
      name: minio-test
      type: s3
  storageSize: 1Gi
  resources:
    total:
      limits:
        memory: 2Gi
        cpu: 2000m
  template:
    queryFrontend:
      jaegerQuery:
        enabled: true
`

	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testDataUnstructured := &unstructured.Unstructured{}

	_, _, err := decoder.Decode([]byte(dataYAML), nil, testDataUnstructured)
	assert.NoErrorf(t, err, "unable to decode tempostacks")

	tests := []struct {
		name             string
		gvrList          map[schema.GroupVersionResource]string
		dataUnstructured *unstructured.Unstructured
		want             []record.Record
		wantErrs         []error
	}{
		{
			name:             "Successfully collects TempoStacks",
			gvrList:          map[schema.GroupVersionResource]string{tempoStackResource: "TempoStackList"},
			dataUnstructured: testDataUnstructured,
			want: []record.Record{
				{
					Name: "config/tempo.grafana.com/openshift-operators/simplest",
					Item: record.ResourceMarshaller{Resource: testDataUnstructured},
				},
			},
			wantErrs: nil,
		},
	}
	for _, test := range tests {
		tt := test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), tt.gvrList)
			_, err := dynamicClient.Resource(tempoStackResource).
				Namespace("openshift-operators").
				Create(ctx, tt.dataUnstructured, metav1.CreateOptions{})
			assert.NoErrorf(t, err, "unable to create fake tempostacks")

			got, gotErrs := gatherTempoStackCR(ctx, dynamicClient)
			assert.Equalf(t, tt.want, got, "unexpected records while gathering tempostacks")
			assert.Equalf(t, tt.wantErrs, gotErrs, "unexpected errors while gathering tempostacks")
		})
	}
}
