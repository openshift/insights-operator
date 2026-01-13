package status

import (
	"context"
	"testing"

	insightsv1 "github.com/openshift/api/insights/v1"
	insightsFakeCli "github.com/openshift/client-go/insights/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProgressingDataGatherCondition(t *testing.T) {
	tests := []struct {
		name                         string
		gatheringReason              string
		expectedProgressingCondition metav1.Condition
	}{
		{
			name:            "Progressing condition running",
			gatheringReason: GatheringReason,
			expectedProgressingCondition: metav1.Condition{
				Type:    Progressing,
				Status:  metav1.ConditionTrue,
				Reason:  GatheringReason,
				Message: GatheringMessage,
			},
		},
		{
			name:            "Progressing condition completed",
			gatheringReason: GatheringSucceededReason,
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
			createdCondition := ProgressingCondition(tt.gatheringReason)
			assert.Equal(t, tt.expectedProgressingCondition.Status, createdCondition.Status)
			assert.Equal(t, tt.expectedProgressingCondition.Reason, createdCondition.Reason)
			assert.Equal(t, tt.expectedProgressingCondition.Message, createdCondition.Message)
		})
	}
}

func TestUpdateDataGatherConditions(t *testing.T) {
	tests := []struct {
		name               string
		dataGather         *insightsv1.DataGather
		updatedCondition   []metav1.Condition
		expectedConditions []metav1.Condition
	}{
		{
			name: "All conditions unknown and DataRecorcded condition updated",
			dataGather: &insightsv1.DataGather{
				Status: insightsv1.DataGatherStatus{
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
			dataGather: &insightsv1.DataGather{
				Status: insightsv1.DataGatherStatus{
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
			dataGather: &insightsv1.DataGather{
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
						DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
						DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
					},
				},
			},
			updatedCondition: []metav1.Condition{
				RemoteConfigurationValidCondition(metav1.ConditionTrue, "Available", "test"),
				ProgressingCondition(GatheringSucceededReason),
				DataProcessedCondition(metav1.ConditionUnknown, "testUpdated", ""),
			},
			expectedConditions: []metav1.Condition{
				DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
				DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
				DataProcessedCondition(metav1.ConditionUnknown, "testUpdated", ""),
				RemoteConfigurationValidCondition(metav1.ConditionTrue, "Available", "test"),
				ProgressingCondition(GatheringSucceededReason),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset(tt.dataGather)
			updatedDG, err := UpdateDataGatherConditions(
				context.Background(),
				cs.InsightsV1(),
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
