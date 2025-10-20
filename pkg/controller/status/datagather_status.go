package status

import (
	"context"

	insightsv1 "github.com/openshift/api/insights/v1"
	insightsv1client "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
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

	// Progressing Condition
	Progressing                 = "Progressing"
	DataGatheringPendingReason  = "DataGatherPending"
	DataGatheringPendingMessage = "The gathering has not started yet"
	GatheringReason             = "Gathering"
	GatheringMessage            = "The gathering is running"
	GatheringSucceededReason    = "GatheringSucceeded"
	GatheringSucceededMessage   = "The gathering successfully finished."
	GatheringFailedReason       = "GatheringFailed"
	GatheringFailedMessage      = "The gathering failed."
)

// ProgressingCondition returns new "ProgressingCondition" status condition with provided status, reason and message
func ProgressingCondition(gatheringReason string) metav1.Condition {
	progressingStatus := metav1.ConditionFalse

	var progressingMessage string
	switch gatheringReason {
	case DataGatheringPendingReason:
		progressingMessage = DataGatheringPendingMessage
	case GatheringReason:
		progressingStatus = metav1.ConditionTrue
		progressingMessage = GatheringMessage
	case GatheringSucceededReason:
		progressingMessage = GatheringSucceededMessage
	case GatheringFailedReason:
		progressingMessage = GatheringFailedMessage
	}

	return metav1.Condition{
		Type:               Progressing,
		LastTransitionTime: metav1.Now(),
		Status:             progressingStatus,
		Reason:             gatheringReason,
		Message:            progressingMessage,
	}
}

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

// TODO: rename, it does not update State anymore, but Progressing conditions,
// so maybe even move it to the conditions package?
// UpdateProgressingCondition updates status' time attributes, state and conditions
// of the provided DataGather resource
func UpdateProgressingCondition(ctx context.Context,
	insightsClient insightsv1client.InsightsV1Interface,
	dataGatherCR *insightsv1.DataGather,
	dataGatherName string,
	gatheringState string,
) (*insightsv1.DataGather, error) {
	var err error
	if dataGatherCR == nil {
		dataGatherCR, err = insightsClient.DataGathers().Get(ctx, dataGatherName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Failed to get DataGather resource %s: %v", dataGatherName, err)
			return nil, err
		}
	}

	switch gatheringState {
	case GatheringSucceededReason, GatheringFailedReason:
		if dataGatherCR.Status.FinishTime == (metav1.Time{}) {
			dataGatherCR.Status.FinishTime = metav1.Now()
		}
	case GatheringReason:
		if dataGatherCR.Status.StartTime == (metav1.Time{}) {
			dataGatherCR.Status.StartTime = metav1.Now()
		}
	case DataGatheringPendingReason:
		// no op
	}

	updatedDataGather, err := UpdateDataGatherConditions(
		ctx, insightsClient, dataGatherCR, ProgressingCondition(gatheringState),
	)
	if err != nil {
		klog.Errorf("Failed to update DataGather resource %s conditions: %v", dataGatherCR.Name, err)
		return nil, err
	}

	return updatedDataGather, nil
}

// GetConditionByType tries to get the condition with the provided condition status
// from the provided "datagather" resource. Returns nil when no condition is found.
func GetConditionByType(dataGather *insightsv1.DataGather, conType string) *metav1.Condition {
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
	insightsClient insightsv1client.InsightsV1Interface,
	dataGather *insightsv1.DataGather, conditions ...metav1.Condition,
) (*insightsv1.DataGather, error) {
	newConditions := make([]metav1.Condition, len(dataGather.Status.Conditions))
	_ = copy(newConditions, dataGather.Status.Conditions)

	for _, condition := range conditions {
		idx := getConditionIndexByType(condition.Type, newConditions)
		if idx != -1 {
			newConditions[idx] = condition
		} else {
			newConditions = append(newConditions, condition)
		}
	}

	dataGather.Status.Conditions = newConditions
	return insightsClient.DataGathers().UpdateStatus(ctx, dataGather, metav1.UpdateOptions{})
}
