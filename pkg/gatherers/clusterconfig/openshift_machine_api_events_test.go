package clusterconfig

import (
	"context"
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_WarningEvents_gatherOpenshiftMachineAPIEvents(t *testing.T) {
	normalEvent := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "normalEvent"},
		LastTimestamp: metav1.Now(),
		Type:          "Normal",
		Reason:        "normal",
	}
	warningEvent := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "warningEvent"},
		LastTimestamp: metav1.Now(),
		Type:          "Normal",
		Reason:        "warning",
	}

	type args struct {
		ctx        context.Context
		coreClient corev1client.CoreV1Interface
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "empty openshift-machine-api events",
			args: args{
				ctx:        context.TODO(),
				coreClient: kubefake.NewSimpleClientset(normalEvent.DeepCopy(), warningEvent.DeepCopy()).CoreV1(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := gatherOpenshiftMachineAPIEvents(tt.args.ctx, tt.args.coreClient, 1*time.Minute)
			if (err != nil) != tt.wantErr {
				t.Errorf("gatherOpenshiftMachineApiEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if reflect.DeepEqual(got, nil) {
				t.Errorf("gatherOpenshiftMachineApiEvents() got nil")
			}
		})
	}
}
