package status

import (
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// OperatorDisabled defines the condition type when the operator's primary function has been disabled
	OperatorDisabled configv1.ClusterStatusConditionType = "Disabled"
	// InsightsUploadDegraded defines the condition type (when set to True) when an archive can't be successfully uploaded
	InsightsUploadDegraded configv1.ClusterStatusConditionType = "UploadDegraded"
	// InsightsDownloadDegraded defines the condition type (when set to True) when the Insights report can't be successfully downloaded
	InsightsDownloadDegraded configv1.ClusterStatusConditionType = "InsightsDownloadDegraded"
	// SCANotAvailable is a condition type providing info about unsuccessful SCA pull attempt from the OCM API
	SCANotAvailable configv1.ClusterStatusConditionType = "SCANotAvailable"
	// ClusterTransferFailed is a condition type providing info about unsuccessful pull attempt of the ClusterTransfer from the OCM API
	// or unsuccessful pull-secret update
	ClusterTransferFailed configv1.ClusterStatusConditionType = "ClusterTransferFailed"
)

type conditionsMap map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition

type conditions struct {
	entryMap conditionsMap
}

func newConditions(cos *configv1.ClusterOperatorStatus, time metav1.Time) *conditions {
	entries := map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
		configv1.OperatorAvailable: {
			Type:               configv1.OperatorAvailable,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
		configv1.OperatorProgressing: {
			Type:               configv1.OperatorProgressing,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
		configv1.OperatorDegraded: {
			Type:               configv1.OperatorDegraded,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
	}

	for _, c := range cos.Conditions {
		entries[c.Type] = c
	}

	return &conditions{
		entryMap: entries,
	}
}

func (c *conditions) setCondition(conditionType configv1.ClusterStatusConditionType,
	status configv1.ConditionStatus, reason, message string, lastTime metav1.Time) {
	originalCondition, ok := c.entryMap[conditionType]
	// if condition is defined and there is not new status then don't update transition time
	if ok && originalCondition.Status == status {
		lastTime = originalCondition.LastTransitionTime
	}

	c.entryMap[conditionType] = configv1.ClusterOperatorStatusCondition{
		Type:               conditionType,
		Reason:             reason,
		Status:             status,
		Message:            message,
		LastTransitionTime: lastTime,
	}
}

func (c *conditions) removeCondition(condition configv1.ClusterStatusConditionType) {
	delete(c.entryMap, condition)
}

func (c *conditions) hasCondition(condition configv1.ClusterStatusConditionType) bool {
	_, ok := c.entryMap[condition]
	return ok
}

func (c *conditions) findCondition(condition configv1.ClusterStatusConditionType) *configv1.ClusterOperatorStatusCondition {
	existing, ok := c.entryMap[condition]
	if ok {
		return &existing
	}
	return nil
}

func (c *conditions) entries() []configv1.ClusterOperatorStatusCondition {
	var res []configv1.ClusterOperatorStatusCondition
	for _, v := range c.entryMap {
		res = append(res, v)
	}
	return res
}
