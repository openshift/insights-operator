package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func Test_gatherMultiClusterEngine(t *testing.T) {
	mceYAML := `
apiVersion: multicluster.openshift.io/v1
kind: MultiClusterEngine
metadata:
  name: multiclusterengine
  resourceVersion: "12345"
  creationTimestamp: "2024-01-01T00:00:00Z"
spec:
  availabilityConfig: High
  targetNamespace: multicluster-engine
  overrides:
    components:
    - name: hypershift
      enabled: true
    - name: managedserviceaccount
      enabled: true
status:
  phase: Available
  currentVersion: 2.5.0
  desiredVersion: 2.5.0
  conditions:
  - type: Available
    status: "True"
    reason: MultiClusterEngineAvailable
    message: All components are available
`

	tests := []struct {
		name          string
		mceYAMLs      []string
		expectedCount int
		expectedName  string
	}{
		{
			name:          "single multiclusterengine resource",
			mceYAMLs:      []string{mceYAML},
			expectedCount: 1,
			expectedName:  "cluster-scoped-resources/multicluster.openshift.io/multiclusterengines/multiclusterengine",
		},
		{
			name:          "no multiclusterengine resources",
			mceYAMLs:      []string{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				runtime.NewScheme(),
				map[schema.GroupVersionResource]string{
					multiClusterEngineGVR: "MultiClusterEngineList",
				},
			)

			for _, mceYAMLStr := range tt.mceYAMLs {
				decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
				mce := &unstructured.Unstructured{}

				_, _, err := decUnstructured.Decode([]byte(mceYAMLStr), nil, mce)
				assert.NoError(t, err, "unable to decode MultiClusterEngine YAML")

				_, err = dynamicClient.Resource(multiClusterEngineGVR).Create(
					context.Background(),
					mce,
					metav1.CreateOptions{},
				)
				assert.NoError(t, err, "unable to create fake MultiClusterEngine resource")
			}

			records, errs := gatherMultiClusterEngine(context.Background(), dynamicClient)

			assert.Empty(t, errs, "unexpected errors when gathering MultiClusterEngine")
			assert.Len(t, records, tt.expectedCount)

			if tt.expectedName != "" && len(records) > 0 {
				assert.Equal(t, tt.expectedName, records[0].Name)
			}
		})
	}
}

func Test_gatherMultiClusterEngine_CRDNotFound(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			multiClusterEngineGVR: "MultiClusterEngineList",
		},
	)
	dynamicClient.PrependReactor("list", "multiclusterengines", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewNotFound(multiClusterEngineGVR.GroupResource(), "")
	})

	records, errs := gatherMultiClusterEngine(context.Background(), dynamicClient)

	assert.Empty(t, errs, "CRD not found should not produce errors")
	assert.Empty(t, records, "CRD not found should not produce records")
}
