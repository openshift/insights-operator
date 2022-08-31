package clusterconfig

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_WarningEvents_gatherOpenshiftMachineAPIEvents(t *testing.T) {
	normalEvent1 := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "normalEvent1", Namespace: "openshift-machine-api"},
		LastTimestamp: metav1.Now(),
		Type:          "Normal",
		Reason:        "normal",
	}
	warningEvent1 := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "warningEvent1", Namespace: "openshift-machine-api"},
		LastTimestamp: metav1.Now(),
		Type:          "Warning",
		Reason:        "warning",
	}
	normalEvent2 := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "normalEvent2", Namespace: "openshift-machine-api"},
		LastTimestamp: metav1.Now(),
		Type:          "Normal",
		Reason:        "normal",
	}
	warningEvent2 := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "warningEvent2", Namespace: "openshift-machine-api"},
		LastTimestamp: metav1.Now(),
		Type:          "Warning",
		Reason:        "warning",
	}
	warningEvent3 := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "warningEvent3", Namespace: "openshift-machine-api"},
		LastTimestamp: metav1.Time{},
		Type:          "Warning",
		Reason:        "warning",
	}
	normalEvent3 := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "normalEvent3", Namespace: "openshift-machine-api"},
		LastTimestamp: metav1.Time{},
		Type:          "Normal",
		Reason:        "normal",
	}
	var events v1.EventList
	events.Items = append(events.Items, warningEvent1, warningEvent2)
	compactedEvents := eventListToCompactedEventList(&events)

	type args struct {
		ctx        context.Context
		coreClient corev1client.CoreV1Interface
	}
	test := struct {
		name    string
		args    args
		wantErr bool
		want    []record.Record
	}{
		name: "openshift-machine-api warning events",
		args: args{
			ctx: context.TODO(),
			coreClient: kubefake.NewSimpleClientset(&warningEvent1, &normalEvent1, &normalEvent2,
				&warningEvent2, &warningEvent3, &normalEvent3).CoreV1(),
		},
		wantErr: false,
		want:    []record.Record{{Name: "events/openshift-machine-api", Item: record.JSONMarshaller{Object: &compactedEvents}}},
	}

	t.Run(test.name, func(t *testing.T) {
		got, err := gatherOpenshiftMachineAPIEvents(test.args.ctx, test.args.coreClient, 1*time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, test.want, got)
	})
}
