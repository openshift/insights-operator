package clusterconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_GatherCephCluster(t *testing.T) {
	var cephClusterYAML = `
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ocs-storagecluster-cephcluster
  namespace: openshift-storage
status:
  attribute1: value1
  ceph:
    phase: Ready
    health: HEALTH_ERROR
`

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		cephClustereResource: "CephClustersList",
	})
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testCephCluster := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(cephClusterYAML), nil, testCephCluster)
	if err != nil {
		t.Fatal("unable to decode cephcluster ", err)
	}
	_, err = dynamicClient.Resource(cephClustereResource).
		Namespace("openshift-storage").
		Create(context.Background(), testCephCluster, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake cephcluster ", err)
	}

	records, errs := gatherCephCluster(context.Background(), dynamicClient)
	assert.Len(t, errs, 0, "unexpected errors when gathering CephCluster resource")
	assert.Len(t, records, 1)

	recordData, err := records[0].Item.Marshal(context.Background())
	assert.NoError(t, err)
	var recordedCephCluster map[string]interface{}
	err = json.Unmarshal(recordData, &recordedCephCluster)
	assert.NoError(t, err)

	a1, ok, err := unstructured.NestedString(recordedCephCluster, "attribute1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "value1", a1)

	ceph, ok, err := unstructured.NestedMap(recordedCephCluster, "ceph")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Ready", ceph["phase"])
	assert.Equal(t, "HEALTH_ERROR", ceph["health"])
}
