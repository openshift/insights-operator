package status

import (
	"context"

	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
	insightsv1alpha1cli "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DataUploaded  = "DataUploaded"
	DataRecorded  = "DataRecorded"
	DataProcessed = "DataProcessed"
)

// DataUploadedCondition returns new "DataUploaded" status condition with provided status, reason and message
func DataUploadedCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               DataUploaded,
		LastTransitionTime: metav1.Now(),
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
}

// DataRecordedCondition returns new "DataRecorded" status condition with provided status, reason and message
func DataRecordedCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               DataRecorded,
		LastTransitionTime: metav1.Now(),
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
}

// DataProcessedCondition returns new "DataProcessed" status condition with provided status, reason and message
func DataProcessedCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               DataProcessed,
		LastTransitionTime: metav1.Now(),
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
}

// updateDataGatherStatus updates status' time attributes, state and conditions
// of the provided DataGather resource
func UpdateDataGatherStatus(ctx context.Context,
	insightsClient insightsv1alpha1cli.InsightsV1alpha1Interface,
	dataGatherCR *insightsv1alpha1.DataGather,
	newState insightsv1alpha1.DataGatherState, conditions []metav1.Condition) (*insightsv1alpha1.DataGather, error) {
	switch newState {
	case insightsv1alpha1.Completed:
		dataGatherCR.Status.FinishTime = metav1.Now()
	case insightsv1alpha1.Failed:
		dataGatherCR.Status.FinishTime = metav1.Now()
	case insightsv1alpha1.Running:
		dataGatherCR.Status.StartTime = metav1.Now()
	case insightsv1alpha1.Pending:
		// no op
	}
	dataGatherCR.Status.State = newState
	if conditions != nil {
		dataGatherCR.Status.Conditions = append(dataGatherCR.Status.Conditions, conditions...)
	}
	return insightsClient.DataGathers().UpdateStatus(ctx, dataGatherCR, metav1.UpdateOptions{})
}

// GetConditionByStatus tries to get the condition with the provided condition status
// from the provided "datagather" resource. Returns nil when no condition is found.
func GetConditionByStatus(dataGather *insightsv1alpha1.DataGather, conStatus string) *metav1.Condition {
	var dataUploadedCon *metav1.Condition
	for i := range dataGather.Status.Conditions {
		con := dataGather.Status.Conditions[i]
		if con.Type == conStatus {
			dataUploadedCon = &con
		}
	}
	return dataUploadedCon
}
