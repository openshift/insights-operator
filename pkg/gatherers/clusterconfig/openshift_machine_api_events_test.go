package clusterconfig

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_WarningEvents_gatherOpenshiftMachineApiEvents(t *testing.T) {
	type args struct {
		ctx        context.Context
		coreClient corev1client.CoreV1Interface
		namespace  string
	}
	tests := []struct {
		name    string
		args    args
		want    []record.Record
		wantErr bool
	}{
		{
			name: "openshift-machine-api events",
			args: args{
				ctx:        context.TODO(),
				coreClient: kubefake.NewSimpleClientset().CoreV1(),
				namespace:  "openshift-machine-api",
			},
			want:    []record.Record{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := gatherOpenshiftMachineApiEvents(tt.args.ctx, tt.args.coreClient, tt.args.namespace, 1*time.Minute)
			if (err != nil) != tt.wantErr {
				t.Errorf("gatherOpenshiftMachineApiEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherNameSpaceEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}
