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

func Test_GatherOpenshiftLogging(t *testing.T) {
	var openshiftLoggingYAML = `
apiVersion: logging.openshift.io/v1
kind: ClusterLogging
metadata:
    name: instance 
    namespace: openshift-logging
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
			name: "empty cluster operator resources",
			args: args{
				ctx: context.TODO(),
				dynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
					openshiftLoggingResource: "ClusterLoggingList",
				}),
			},
			totalRecords:  1,
			recordName:    "config/logging/openshift-logging/instance",
			expectedError: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
			testOpenshiftLoggingResource := &unstructured.Unstructured{}

			_, _, err := decUnstructured.Decode([]byte(openshiftLoggingYAML), nil, testOpenshiftLoggingResource)
			if err != nil {
				t.Fatal("unable to decode clusterlogging ", err)
			}
			_, err = tt.args.dynamicClient.Resource(openshiftLoggingResource).Namespace("openshift-logging").Create(context.Background(), testOpenshiftLoggingResource, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unable to create fake resource %s", err)
			}

			records, errs := gatherOpenshiftLogging(tt.args.ctx, tt.args.dynamicClient)
			if len(errs) > 0 {
				if errs[0].Error() != tt.expectedError.Error() {
					t.Fatalf("unexpected errors: %v", errs[0].Error())
				}
			}
			if len(records) != tt.totalRecords {
				t.Errorf("gatherOpenshiftLogging() got = %v, want %v", len(records), tt.totalRecords)
			}
			if records[0].Name != tt.recordName {
				t.Errorf("gatherOpenshiftLogging() name = %v, want %v ", records[0].Name, tt.recordName)
			}
		})
	}
}
