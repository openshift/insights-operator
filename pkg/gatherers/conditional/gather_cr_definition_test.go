package conditional

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var testYAML1 = `
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
    name: rook-ceph
    namespace: openshift-storage
spec:
    cephVersion:
        image: quay.io/ceph/ceph:v18.2.0
status:
    state: Created
`

var testYAML2 = `
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
    name: rook-ceph-external
    namespace: openshift-storage
spec:
    external:
        enable: true
status:
    state: Connected
`

func TestGatherer_gatherCRDefinition(t *testing.T) {
	type fields struct {
		firingAlerts map[string][]AlertLabels
	}
	type args struct {
		dynamicClient dynamic.Interface
	}

	testParams := GatherCRDefinitionParams{
		AlertName: "CephClusterWarningState",
		Group:     "ceph.rook.io",
		Version:   "v1",
		Resource:  "cephclusters",
	}

	tests := []struct {
		name         string
		firingAlerts map[string][]AlertLabels
		yamlFiles    []string
		wantLen      int
		wantErr      bool
		wantErrLen   int
		checkRecord  func(*testing.T, []any)
	}{
		{
			name: "gather single CR from single namespace",
			firingAlerts: map[string][]AlertLabels{
				"CephClusterWarningState": {{"namespace": "openshift-storage"}},
			},
			yamlFiles:  []string{testYAML1},
			wantLen:    1,
			wantErr:    false,
			wantErrLen: 0,
		},
		{
			name: "gather multiple CRs from single namespace",
			firingAlerts: map[string][]AlertLabels{
				"CephClusterWarningState": {{"namespace": "openshift-storage"}},
			},
			yamlFiles:  []string{testYAML1, testYAML2},
			wantLen:    2,
			wantErr:    false,
			wantErrLen: 0,
		},
		{
			name: "CR not found in namespace",
			firingAlerts: map[string][]AlertLabels{
				"CephClusterWarningState": {{"namespace": "non-existent-namespace"}},
			},
			wantLen:    0,
			wantErr:    false,
			wantErrLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given
			gvr := schema.GroupVersionResource{Group: "ceph.rook.io", Version: "v1", Resource: "cephclusters"}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				runtime.NewScheme(),
				map[schema.GroupVersionResource]string{gvr: "CephClusterList"},
			)
			for i := range tt.yamlFiles {
				createCRResource(t, tt.yamlFiles[i], client, gvr, "openshift-storage")
			}

			// When
			g := &Gatherer{firingAlerts: tt.firingAlerts}
			test, testErr := g.gatherCRDefinition(context.TODO(), testParams, client)

			assert.Len(t, test, tt.wantLen)
			if tt.wantErr {
				assert.Len(t, testErr, tt.wantErrLen)
			} else {
				assert.Len(t, testErr, 0)
			}

			if tt.checkRecord != nil && len(test) > 0 {
				var recordNames []any
				for _, record := range test {
					recordNames = append(recordNames, record.Name)
				}
				tt.checkRecord(t, recordNames)
			}
		})
	}
}

// Helper function to create CR resources in the fake dynamic client
func createCRResource(t *testing.T, yamlDef string, client dynamic.Interface, gvr schema.GroupVersionResource, namespace string) {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	crObj := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(yamlDef), nil, crObj)
	assert.NoError(t, err, "Failed to decode CR YAML definition")

	_, err = client.Resource(gvr).Namespace(namespace).Create(context.Background(), crObj, metav1.CreateOptions{})
	assert.NoError(t, err, "Failed to create fake CR resource")
}
