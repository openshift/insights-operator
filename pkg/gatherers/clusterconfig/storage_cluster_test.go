//nolint: dupl
package clusterconfig

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_GatherOpenshiftStorage(t *testing.T) {
	var openshiftStorageYAML = `
apiVersion: ocs.openshift.io/v1
kind: StorageCluster
metadata:
  name: ocs-storagecluster
  namespace: openshift-storage
`
	type args struct {
		ctx           context.Context
		dynamicClient dynamic.Interface
	}

	tests := []struct {
		name          string
		args          args
		totalRecords  int
		recordName    string
		expectedError error
	}{
		{
			name: "check for storagecluster resource",
			args: args{
				ctx: context.TODO(),
				dynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
					storageClusterResource: "StorageClusterList",
				}),
			},
			totalRecords:  1,
			recordName:    "config/storage/openshift-storage/storageclusters/ocs-storagecluster",
			expectedError: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
			testOpenshiftStorageResource := &unstructured.Unstructured{}

			_, _, err := decUnstructured.Decode([]byte(openshiftStorageYAML), nil, testOpenshiftStorageResource)
			if err != nil {
				t.Fatal("unable to decode storagecluster ", err)
			}
			_, err = tt.args.dynamicClient.
				Resource(storageClusterResource).
				Namespace("openshift-storage").
				Create(context.Background(), testOpenshiftStorageResource, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unable to create fake resource %s", err)
			}

			records, errs := gatherStorageCluster(tt.args.ctx, tt.args.dynamicClient)
			if len(errs) > 0 {
				if errs[0].Error() != tt.expectedError.Error() {
					t.Fatalf("unexpected errors: %v", errs[0].Error())
				}
			}
			if len(records) != tt.totalRecords {
				t.Errorf("gatherStorageCluster() got = %v, want %v", len(records), tt.totalRecords)
			}
			if records[0].Name != tt.recordName {
				t.Errorf("gatherStorageCluster() name = %v, want %v ", records[0].Name, tt.recordName)
			}
		})
	}
}
