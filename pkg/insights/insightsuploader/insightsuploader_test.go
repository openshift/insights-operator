package insightsuploader

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_updateClusterOperatorLastReportTime(t *testing.T) {
	tests := []struct {
		name            string
		clusterOperator *configv1.ClusterOperator
		expErr          bool
		expTimeSet      bool
		expErrContains  string
	}{
		{
			name: "Successfully updates lastReportTime on existing ClusterOperator",
			clusterOperator: &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Extension: runtime.RawExtension{
						Raw: []byte(`{}`),
					},
				},
			},
			expErr:     false,
			expTimeSet: true,
		},
		{
			name: "Updates lastReportTime when extension has existing data",
			clusterOperator: &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Extension: runtime.RawExtension{
						Raw: []byte(`{"lastReportTime":"2024-01-01T00:00:00Z"}`),
					},
				},
			},
			expErr:     false,
			expTimeSet: true,
		},
		{
			name:            "Returns error when ClusterOperator doesn't exist",
			clusterOperator: nil,
			expErr:          true,
			expTimeSet:      false,
			expErrContains:  "not found",
		},
		{
			name: "Handles invalid JSON in extension by overwriting",
			clusterOperator: &configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Extension: runtime.RawExtension{
						Raw: []byte(`{invalid json}`),
					},
				},
			},
			expErr:     false,
			expTimeSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var operators []runtime.Object
			if tt.clusterOperator != nil {
				operators = append(operators, tt.clusterOperator)
			}
			fakeClient := configfake.NewSimpleClientset(operators...)

			timeBefore := time.Now().UTC().Truncate(time.Second)

			ctx := context.Background()
			err := updateClusterOperatorLastReportTime(ctx, fakeClient.ConfigV1())

			timeAfter := time.Now().UTC().Truncate(time.Second).Add(time.Second)

			if tt.expErr {
				assert.Error(t, err)
				if tt.expErrContains != "" {
					assert.Contains(t, err.Error(), tt.expErrContains)
				}
				return
			}

			assert.NoError(t, err)

			if tt.expTimeSet {
				reportTime := getLastReportTime(t, ctx, fakeClient).UTC()

				assert.True(t, !reportTime.Before(timeBefore),
					"LastReportTime (%v) should be at or after timeBefore (%v)", reportTime, timeBefore)
				assert.True(t, !reportTime.After(timeAfter),
					"LastReportTime (%v) should be at or before timeAfter (%v)", reportTime, timeAfter)
			}
		})
	}
}

func Test_updateClusterOperatorLastReportTime_TimestampProgression(t *testing.T) {
	clusterOperator := createBasicClusterOperator()

	fakeClient := configfake.NewSimpleClientset(clusterOperator)
	ctx := context.Background()

	err := updateClusterOperatorLastReportTime(ctx, fakeClient.ConfigV1())
	assert.NoError(t, err)

	firstTime := getLastReportTime(t, ctx, fakeClient)

	time.Sleep(10 * time.Millisecond)

	err = updateClusterOperatorLastReportTime(ctx, fakeClient.ConfigV1())
	assert.NoError(t, err)

	secondTime := getLastReportTime(t, ctx, fakeClient)

	assert.True(t, !secondTime.Before(firstTime),
		"Second timestamp (%v) should be at or after first timestamp (%v)", secondTime, firstTime)
}

// Create a basic ClusterOperator for testing
func createBasicClusterOperator() *configv1.ClusterOperator {
	return &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "insights",
		},
		Status: configv1.ClusterOperatorStatus{},
	}
}

// Retrieve and unmarshal the LastReportTime
func getLastReportTime(t *testing.T, ctx context.Context, client *configfake.Clientset) time.Time {
	updatedCo, err := client.ConfigV1().ClusterOperators().Get(ctx, "insights", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, updatedCo.Status.Extension.Raw)

	var reported status.Reported
	err = json.Unmarshal(updatedCo.Status.Extension.Raw, &reported)
	assert.NoError(t, err)
	assert.False(t, reported.LastReportTime.IsZero())

	return reported.LastReportTime.Time
}
