package status

import (
	"context"

	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
	insightsv1alpha1cli "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DataUploaded Condition
	DataUploaded             = "DataUploaded"
	NoUploadYetReason        = "NoUploadYet"
	NoDataGatheringYetReason = "NoDataGatheringYet"

	// DataRecorded Condition
	DataRecorded          = "DataRecorded"
	RecordingFailedReason = "RecordingFailed"

	// DataProcessed Condition
	DataProcessed             = "DataProcessed"
	NothingToProcessYetReason = "NothingToProcessYet"
	ProcessedReason           = "Processed"

	// RemoteConfiguration Condition
	RemoteConfNotValidatedYet = "NoValidationYet"
	RemoteConfNotRequestedYet = "RemoteConfigNotRequestedYet"
	UnknownReason             = "Unknown"
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

// RemoteConfigurationAvailableCondition returns new "RemoteConfigurationAvailable" status condition with provided
// status, reason and message
func RemoteConfigurationAvailableCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               string(RemoteConfigurationAvailable),
		LastTransitionTime: metav1.Now(),
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
}

// RemoteConfigurationValidCondition returns new "RemoteConfigurationValid" status condition with provided
// status, reason and message
func RemoteConfigurationValidCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               string(RemoteConfigurationValid),
		LastTransitionTime: metav1.Now(),
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
}

// UpdateDataGatherState updates status' time attributes, state and conditions
// of the provided DataGather resource
func UpdateDataGatherState(ctx context.Context,
	insightsClient insightsv1alpha1cli.InsightsV1alpha1Interface,
	dataGatherCR *insightsv1alpha1.DataGather,
	newState insightsv1alpha1.DataGatherState,
) (*insightsv1alpha1.DataGather, error) {
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
	return insightsClient.DataGathers().UpdateStatus(ctx, dataGatherCR, metav1.UpdateOptions{})
}

// GetConditionByType tries to get the condition with the provided condition status
// from the provided "datagather" resource. Returns nil when no condition is found.
func GetConditionByType(dataGather *insightsv1alpha1.DataGather, conType string) *metav1.Condition {
	var c *metav1.Condition
	for i := range dataGather.Status.Conditions {
		con := dataGather.Status.Conditions[i]
		if con.Type == conType {
			c = &con
		}
	}
	return c
}

// getConditionIndexByType tries to find an index of the condition with the provided type.
// If no match is found, it returns -1.
func getConditionIndexByType(conType string, conditions []metav1.Condition) int {
	idx := -1
	for i := range conditions {
		con := conditions[i]
		if con.Type == conType {
			idx = i
		}
	}
	return idx
}

// UpdateDataGatherConditions updates the conditions of the provided dataGather resource with provided
// condition
func UpdateDataGatherConditions(ctx context.Context,
	insightsClient insightsv1alpha1cli.InsightsV1alpha1Interface,
	dataGather *insightsv1alpha1.DataGather, condition *metav1.Condition,
) (*insightsv1alpha1.DataGather, error) {
	newConditions := make([]metav1.Condition, len(dataGather.Status.Conditions))
	_ = copy(newConditions, dataGather.Status.Conditions)
	idx := getConditionIndexByType(condition.Type, newConditions)
	if idx != -1 {
		newConditions[idx] = *condition
	} else {
		newConditions = append(newConditions, *condition)
	}
	dataGather.Status.Conditions = newConditions
	return insightsClient.DataGathers().UpdateStatus(ctx, dataGather, metav1.UpdateOptions{})
}
