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

func TestUpdateDataGatherConditions(t *testing.T) {
	tests := []struct {
		name                  string
		dataGather            *v1alpha1.DataGather
		updatedCondition      metav1.Condition
		expectedDataRecorded  metav1.Condition
		expectedDataProcessed metav1.Condition
		expectedDataUploaded  metav1.Condition
	}{
		{
			name: "All conditions unknown and DataRecorcded condition updated",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
						DataRecordedCondition(metav1.ConditionUnknown, "test", ""),
						DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
					},
				},
			},
			updatedCondition:      DataRecordedCondition(metav1.ConditionTrue, "Recorded", "test"),
			expectedDataRecorded:  DataRecordedCondition(metav1.ConditionTrue, "Recorded", "test"),
			expectedDataProcessed: DataProcessedCondition(metav1.ConditionUnknown, "test", ""),
			expectedDataUploaded:  DataUploadedCondition(metav1.ConditionUnknown, "test", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset(tt.dataGather)
			updatedDG, err := UpdateDataGatherConditions(context.Background(), cs.InsightsV1alpha1(), tt.dataGather, &tt.updatedCondition)
			assert.NoError(t, err)
			dataRecorded := GetConditionByType(updatedDG, DataRecorded)
			assert.Equal(t, tt.expectedDataRecorded.Status, dataRecorded.Status)
			assert.Equal(t, tt.expectedDataRecorded.Reason, dataRecorded.Reason)
			assert.Equal(t, tt.expectedDataRecorded.Message, dataRecorded.Message)

			dataUploaded := GetConditionByType(updatedDG, DataUploaded)
			assert.Equal(t, tt.expectedDataUploaded.Status, dataUploaded.Status)
			assert.Equal(t, tt.expectedDataUploaded.Reason, dataUploaded.Reason)
			assert.Equal(t, tt.expectedDataUploaded.Message, dataUploaded.Message)

			dataProcessed := GetConditionByType(updatedDG, DataProcessed)
			assert.Equal(t, tt.expectedDataProcessed.Status, dataProcessed.Status)
			assert.Equal(t, tt.expectedDataProcessed.Reason, dataProcessed.Reason)
			assert.Equal(t, tt.expectedDataProcessed.Message, dataProcessed.Message)
		})
	}
}
