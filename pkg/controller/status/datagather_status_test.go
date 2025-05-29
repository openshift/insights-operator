package status

import (
	"context"
	"testing"

	"github.com/openshift/api/insights/v1alpha1"
	insightsFakeCli "github.com/openshift/client-go/insights/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateDataGatherState(t *testing.T) {
	tests := []struct {
		name       string
		dataGather *v1alpha1.DataGather
		dgState    v1alpha1.DataGatherState
	}{
		{
			name: "updating DataGather to completed state",
			dataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-datagather-1",
				},
			},
			dgState: v1alpha1.Completed,
		},
		{
			name: "updating DataGather to failed state",
			dataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-datagather-1",
				},
			},
			dgState: v1alpha1.Failed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset(tt.dataGather)
			updatedDG, err := UpdateDataGatherState(context.Background(), cs.InsightsV1alpha1(),
				tt.dataGather, tt.dgState)
			assert.NoError(t, err)
			assert.NotNil(t, updatedDG.Status.StartTime)
			assert.NotNil(t, updatedDG.Status.FinishTime)
		})
	}
}

func TestProgressingDataGatherCondition(t *testing.T) {
	tests := []struct {
		name                         string
		state                        v1alpha1.DataGatherState
		expectedProgressingCondition metav1.Condition
	}{
		{
			name:  "Progressing condition running",
			state: v1alpha1.Running,
			expectedProgressingCondition: metav1.Condition{
				Type:    Progressing,
				Status:  metav1.ConditionTrue,
				Reason:  GatheringReason,
				Message: GatheringMessage,
			},
		},
		{
			name:  "Progressing condition completed",
			state: v1alpha1.Completed,
			expectedProgressingCondition: metav1.Condition{
				Type:    Progressing,
				Status:  metav1.ConditionFalse,
				Reason:  GatheringSucceededReason,
				Message: GatheringSucceededMessage,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdCondition := ProgressingCondition(tt.state)
			assert.Equal(t, tt.expectedProgressingCondition.Status, createdCondition.Status)
			assert.Equal(t, tt.expectedProgressingCondition.Reason, createdCondition.Reason)
			assert.Equal(t, tt.expectedProgressingCondition.Message, createdCondition.Message)
		})
	}
}

func TestUpdateDataGatherConditions(t *testing.T) {
	tests := []struct {
		name               string
		dataGather         *v1alpha1.DataGather
		updatedCondition   []metav1.Condition
		expectedConditions []metav1.Condition
	}{
		{
			name: "All conditions unknown and DataRecorcded condition updated",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
						DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
						DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
						RemoteConfigurationValidCondition(metav1.ConditionUnknown, "test", ""),
					},
				},
			},
			updatedCondition: []metav1.Condition{DataRecordedCondition(metav1.ConditionTrue, "Recorded", "test")},
			expectedConditions: []metav1.Condition{
				DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
				DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
				RemoteConfigurationValidCondition(metav1.ConditionUnknown, "test", ""),
				DataRecordedCondition(metav1.ConditionTrue, "Recorded", "test"),
			},
		},
		{
			name: "Updating non-existing condition appends the condition",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
						DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
						DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
					},
				},
			},
			updatedCondition: []metav1.Condition{RemoteConfigurationValidCondition(metav1.ConditionTrue, "Available", "test")},
			expectedConditions: []metav1.Condition{
				DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
				DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
				DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
				RemoteConfigurationValidCondition(metav1.ConditionTrue, "Available", "test"),
			},
		},
		{
			name: "Updating multiple condition appends or updates the condition",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
						DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
						DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
					},
				},
			},
			updatedCondition: []metav1.Condition{
				RemoteConfigurationValidCondition(metav1.ConditionTrue, "Available", "test"),
				ProgressingCondition(v1alpha1.Completed),
				DataProcessedCondition(metav1.ConditionUnknown, "testUpdated", ""),
			},
			expectedConditions: []metav1.Condition{
				DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
				DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
				DataProcessedCondition(metav1.ConditionUnknown, "testUpdated", ""),
				RemoteConfigurationValidCondition(metav1.ConditionTrue, "Available", "test"),
				ProgressingCondition(v1alpha1.Completed),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset(tt.dataGather)
			updatedDG, err := UpdateDataGatherConditions(
				context.Background(),
				cs.InsightsV1alpha1(),
				tt.dataGather,
				tt.updatedCondition...,
			)
			assert.NoError(t, err)

			assert.Len(t, tt.expectedConditions, len(updatedDG.Status.Conditions))

			for _, expectedCondition := range tt.expectedConditions {
				conditionIndex := getConditionIndexByType(expectedCondition.Type, updatedDG.Status.Conditions)
				assert.Equal(t, expectedCondition.Status, updatedDG.Status.Conditions[conditionIndex].Status)
				assert.Equal(t, expectedCondition.Reason, updatedDG.Status.Conditions[conditionIndex].Reason)
				assert.Equal(t, expectedCondition.Message, updatedDG.Status.Conditions[conditionIndex].Message)
			}
		})
	}
}
