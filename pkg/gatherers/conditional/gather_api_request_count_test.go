package conditional

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var apiRequestCountYAML1 = `
apiVersion: apiserver.openshift.io/v1
kind: APIRequestCount
metadata:
    name: test1.v1beta2.testapi.org
status:
    currentHour:
        requestCount: 12
    requestCount: 13
    removedInRelease: "14.15"
`

var apiRequestCountYAML2 = `
apiVersion: apiserver.openshift.io/v1
kind: APIRequestCount
metadata:
    name: test2.v1beta1.testapi.org
status:
    currentHour:
        requestCount: 2
    requestCount: 3
    removedInRelease: "1.123"
`

func Test_GatherAPIRequestCount(t *testing.T) {
	gatherer := Gatherer{
		firingAlerts: map[string][]AlertLabels{
			"alertA": {
				{
					"alertname": "alertA",
					"resource":  "test1",
					"group":     "testapi.org",
					"version":   "v1beta2",
				},
				{
					"alertname": "alertA",
					"resource":  "test2",
					"group":     "testapi.org",
					"version":   "v1beta1",
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{Group: "apiserver.openshift.io", Version: "v1", Resource: "apirequestcounts"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "APIRequestCountsList",
	})
	createResource(t, apiRequestCountYAML1, client, gvr)
	createResource(t, apiRequestCountYAML2, client, gvr)
	records, errs := gatherer.gatherAPIRequestCounts(context.Background(), client, "alertA")
	assert.Empty(t, errs, "Unexpected errors during gathering of API request counts")
	assert.Len(t, records, 1, "Unexpected number of records")

	// check gathered data
	a, ok := records[0].Item.(record.JSONMarshaller).Object.([]APIRequestCount)
	assert.True(t, ok, "Failed to convert")
	assert.Len(t, a, 2, "Unexpected number of alerts")
	assert.Equal(t, a[0].ResourceName, "test1.v1beta2.testapi.org")
	assert.Equal(t, a[0].LastDayRequestCount, int64(12))
	assert.Equal(t, a[0].TotalRequestCount, int64(13))
	assert.Equal(t, a[0].RemovedInRelease, "14.15")

	assert.Equal(t, a[1].ResourceName, "test2.v1beta1.testapi.org")
	assert.Equal(t, a[1].LastDayRequestCount, int64(2))
	assert.Equal(t, a[1].TotalRequestCount, int64(3))
	assert.Equal(t, a[1].RemovedInRelease, "1.123")
}

func createResource(t *testing.T, yamlDef string, client dynamic.Interface, gvr schema.GroupVersionResource) {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testAPIRequestCountObj := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(yamlDef), nil, testAPIRequestCountObj)
	assert.NoError(t, err, "Failed to decode API request count YAML definition")

	_, err = client.Resource(gvr).Create(context.Background(), testAPIRequestCountObj, metav1.CreateOptions{})
	assert.NoError(t, err, "Failed to create fake API request count resource")
}
