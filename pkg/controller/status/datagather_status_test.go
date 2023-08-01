package status

import (
	"context"
	"testing"

	"github.com/openshift/api/insights/v1alpha1"
	insightsFakeCli "github.com/openshift/client-go/insights/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateDataGatherStatus(t *testing.T) {
	tests := []struct {
		name                     string
		dataGather               *v1alpha1.DataGather
		dgState                  v1alpha1.DataGatherState
		updatingConditions       []metav1.Condition
		expectedDataRecordedCon  metav1.Condition
		expectedDataUploadedCon  metav1.Condition
		expectedDataProcessedCon metav1.Condition
	}{
		{
			name: "updating DataGather to completed state",
			dataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-datagather-1",
				},
			},
			dgState: v1alpha1.Completed,
			updatingConditions: []metav1.Condition{
				DataRecordedCondition(metav1.ConditionTrue, "AsExpected", ""),
				DataUploadedCondition(metav1.ConditionTrue, "HttpStatus200", "testing message"),
				DataProcessedCondition(metav1.ConditionTrue, "Processed", ""),
			},
			expectedDataRecordedCon: metav1.Condition{
				Type:   DataRecorded,
				Status: metav1.ConditionTrue,
				Reason: "AsExpected",
			},
			expectedDataUploadedCon: metav1.Condition{
				Type:    DataUploaded,
				Status:  metav1.ConditionTrue,
				Reason:  "HttpStatus200",
				Message: "testing message",
			},
			expectedDataProcessedCon: metav1.Condition{
				Type:   DataProcessed,
				Status: metav1.ConditionTrue,
				Reason: "Processed",
			},
		},
		{
			name: "updating DataGather to failed state",
			dataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-datagather-1",
				},
			},
			dgState: v1alpha1.Failed,
			updatingConditions: []metav1.Condition{
				DataRecordedCondition(metav1.ConditionFalse, "Failure", "testing error message"),
				DataUploadedCondition(metav1.ConditionFalse, "HttpStatus403", "testing message"),
				DataProcessedCondition(metav1.ConditionFalse, "Failure", "testing error message"),
			},
			expectedDataRecordedCon: metav1.Condition{
				Type:    DataRecorded,
				Status:  metav1.ConditionFalse,
				Reason:  "Failure",
				Message: "testing error message",
			},
			expectedDataUploadedCon: metav1.Condition{
				Type:    DataUploaded,
				Status:  metav1.ConditionFalse,
				Reason:  "HttpStatus403",
				Message: "testing message",
			},
			expectedDataProcessedCon: metav1.Condition{
				Type:    DataProcessed,
				Status:  metav1.ConditionFalse,
				Reason:  "Failure",
				Message: "testing error message",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset()
			createdDG, err := cs.InsightsV1alpha1().DataGathers().Create(context.Background(), tt.dataGather, metav1.CreateOptions{})
			assert.NoError(t, err)
			updatedDG, err := UpdateDataGatherStatus(context.Background(), cs.InsightsV1alpha1(),
				createdDG, tt.dgState, tt.updatingConditions)
			assert.NoError(t, err)
			assert.NotNil(t, updatedDG.Status.StartTime)
			assert.NotNil(t, updatedDG.Status.FinishTime)

			dataRecordedCon := GetConditionByStatus(updatedDG, DataRecorded)
			assert.NotNil(t, dataRecordedCon)
			assert.Equal(t, tt.expectedDataRecordedCon.Reason, dataRecordedCon.Reason)
			assert.Equal(t, tt.expectedDataRecordedCon.Status, dataRecordedCon.Status)
			assert.Equal(t, tt.expectedDataRecordedCon.Message, dataRecordedCon.Message)

			dataUploadedCon := GetConditionByStatus(updatedDG, DataUploaded)
			assert.NotNil(t, dataRecordedCon)
			assert.Equal(t, tt.expectedDataUploadedCon.Reason, dataUploadedCon.Reason)
			assert.Equal(t, tt.expectedDataUploadedCon.Status, dataUploadedCon.Status)
			assert.Equal(t, tt.expectedDataUploadedCon.Message, dataUploadedCon.Message)

			dataProcessedCon := GetConditionByStatus(updatedDG, DataProcessed)
			assert.NotNil(t, dataRecordedCon)
			assert.Equal(t, tt.expectedDataProcessedCon.Reason, dataProcessedCon.Reason)
			assert.Equal(t, tt.expectedDataProcessedCon.Status, dataProcessedCon.Status)
			assert.Equal(t, tt.expectedDataProcessedCon.Message, dataProcessedCon.Message)
		})
	}
}
