package clusterconfig

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/openshift/insights-operator/pkg/utils/marshal"

	configFake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"k8s.io/client-go/kubernetes/fake"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getClusterVersion(t *testing.T) {
	lastTimestampEvent := metav1.Time{Time: time.Now().Add(2)}
	compactEvents := eventListToCompactedEventList(&v1.EventList{
		Items: []v1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "openshift-cluster-version",
				},
				Reason:        "Unit test",
				Message:       "This is a unit test event",
				LastTimestamp: lastTimestampEvent,
				Count:         1,
			},
		},
	})

	tests := []struct {
		name                     string
		clusterVersionDefinition *configv1.ClusterVersion
		pods                     *v1.PodList
		events                   *v1.EventList
		wantRecords              []record.Record
		wantErrCounts            int
		interval                 time.Duration
	}{
		{
			name: "successful retrieve node version",
			clusterVersionDefinition: &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "version",
				},
				Spec: configv1.ClusterVersionSpec{
					ClusterID: "cluster-id",
					Channel:   "stable-4.13",
				},
			},
			pods: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "version",
							Namespace: "openshift-cluster-version",
						},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{{RestartCount: 1}},
						},
					},
				},
			},
			events: &v1.EventList{
				Items: []v1.Event{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-cluster-version",
						},
						Reason:        "Unit test",
						Message:       "This is a unit test event",
						LastTimestamp: lastTimestampEvent,
						Count:         1,
					},
				},
			},
			wantRecords: []record.Record{
				{
					Name: "config/version",
					Item: record.ResourceMarshaller{
						Resource: anonymizeClusterVersion(&configv1.ClusterVersion{
							ObjectMeta: metav1.ObjectMeta{
								Name: "version",
							},
							Spec: configv1.ClusterVersionSpec{
								ClusterID: "cluster-id",
								Channel:   "stable-4.13",
							},
						}),
					},
				},
				{
					Name: "config/id",
					Item: marshal.Raw{Str: string("cluster-id")},
				},
				{
					Name: "config/pod/openshift-cluster-version/version",
					Item: record.ResourceMarshaller{Resource: &v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "version",
							Namespace: "openshift-cluster-version",
						},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{{RestartCount: 1}},
						},
					}},
				},
				{
					Name: "events/openshift-cluster-version",
					Item: record.JSONMarshaller{Object: &compactEvents},
				},
			},
			wantErrCounts: 0,
			interval:      time.Second * 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configClient := configFake.NewSimpleClientset(tt.clusterVersionDefinition).ConfigV1()
			coreClient := fake.NewSimpleClientset(tt.pods, tt.events).CoreV1()

			records, errs := getClusterVersion(context.Background(), configClient, coreClient, tt.interval)
			assert.Len(t, errs, tt.wantErrCounts)
			assert.Equal(t, tt.wantRecords, records)
		})
	}
}
