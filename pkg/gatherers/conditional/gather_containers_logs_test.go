package conditional

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

var testAlertManagerFiringAlerts = map[string][]AlertLabels{
	"AlertmanagerFailedToSendAlerts": {
		{
			"pod":       "alertmanager-main-0",
			"namespace": "openshift-monitoring",
		},
	},
}

func TestGatherer_gatherContainersLogs(t *testing.T) {
	ctx := context.TODO()
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alertmanager-main-0",
			Namespace: "openshift-monitoring",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "alertmanager"},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "alertmanager"},
			},
		},
	}

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	_, err := coreClient.Pods("openshift-monitoring").Create(ctx, testPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create fake pod: %v", err)
	}

	tests := []struct {
		name    string
		params  GatherContainersLogsParams
		want    []record.Record
		wantErr []error
	}{
		{
			name: "Can record logs",
			params: GatherContainersLogsParams{
				AlertName: "AlertmanagerFailedToSendAlerts",
				TailLines: 50,
			},
			want: []record.Record{{
				// nolint: lll
				Name:     "conditional/namespaces/openshift-monitoring/pods/alertmanager-main-0/containers/alertmanager/logs/last-50-lines.log",
				Captured: time.Time{},
				Item:     marshal.Raw{Str: "fake logs"},
			}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Gatherer{firingAlerts: testAlertManagerFiringAlerts}
			got, gotErr := g.gatherContainersLogs(ctx, tt.params, coreClient)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherContainersLogs() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(gotErr, tt.wantErr) {
				t.Errorf("gatherContainersLogs() gotErr = %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}
