package clusterconfig

import (
	"context"
	"reflect"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_gatherOpenshiftLogging(t *testing.T) {
	type args struct {
		ctx           context.Context
		dynamicClient dynamic.Interface
		coreClient    corev1client.CoreV1Interface
	}

	clr := schema.GroupVersionResource{
		Group:    "logging.openshift.io",
		Version:  "v1",
		Resource: "clusterloggings",
	}

	tests := []struct {
		name          string
		args          args
		want          []record.Record
		expectedError error
	}{
		{
			name: "empty cluster operator resources",
			args: args{
				ctx: context.TODO(),
				dynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
					clr: "ClusterLoggingList",
				}),
				coreClient: kubefake.NewSimpleClientset().CoreV1(),
			},
			want:          nil,
			expectedError: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := gatherOpenshiftLogging(tt.args.ctx, tt.args.dynamicClient, tt.args.coreClient)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherOpenshiftLogging() got = %v, want %v", got, tt.want)
			}
			if len(errs) > 0 {
				if errs[0].Error() != tt.expectedError.Error() {
					t.Fatalf("unexpected errors: %v", errs[0].Error())
				}
			}
		})
	}
}
