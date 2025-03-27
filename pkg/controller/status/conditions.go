package status

import (
	"sort"

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
	// ClusterTransferAvailable is a condition type providing info about ClusterTransfer controller status
	ClusterTransferAvailable configv1.ClusterStatusConditionType = "ClusterTransferAvailable"
	// SCAAvailable is a condition type providing info about SCA controller status
	SCAAvailable configv1.ClusterStatusConditionType = "SCAAvailable"
	// RemoteConfigurationAvailable is a condition type providing info about remote configuration (conditional gathering)
	// availability
	RemoteConfigurationAvailable configv1.ClusterStatusConditionType = "RemoteConfigurationAvailable"
	// RemoteConfigurationInvalid is a condition type providing info about remote configuration content validity
	RemoteConfigurationValid configv1.ClusterStatusConditionType = "RemoteConfigurationValid"
	// GatheringDisabled is a condition providing information about the disabling of gathering with the API.
	GatheringDisabled configv1.ClusterStatusConditionType = "GatheringDisabled"
)

type conditionsMap map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition

type conditions struct {
	entryMap conditionsMap
}

func newConditions(cos *configv1.ClusterOperatorStatus, time metav1.Time) *conditions {
	entries := map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{ // nolint: dupl
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
		SCAAvailable: {
			Type:               SCAAvailable,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
		ClusterTransferAvailable: {
			Type:               ClusterTransferAvailable,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
		RemoteConfigurationAvailable: {
			Type:               RemoteConfigurationAvailable,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
		RemoteConfigurationValid: {
			Type:               RemoteConfigurationValid,
			Status:             configv1.ConditionUnknown,
			LastTransitionTime: time,
			Reason:             "",
		},
		GatheringDisabled: {
			Type:               GatheringDisabled,
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
	status configv1.ConditionStatus, reason, message string,
) {
	originalCondition, ok := c.entryMap[conditionType]
	transitionTime := metav1.Now()
	// if condition is defined and there is no new status then don't update transition time
	if ok && originalCondition.Status == status {
		transitionTime = originalCondition.LastTransitionTime
	}

	c.entryMap[conditionType] = configv1.ClusterOperatorStatusCondition{
		Type:               conditionType,
		Reason:             reason,
		Status:             status,
		Message:            message,
		LastTransitionTime: transitionTime,
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

// entries returns a sorted list of status conditions from the mapped values.
// The list is sorted by type ClusterStatusConditionType to ensure consistent ordering for deep equal checks.
func (c *conditions) entries() []configv1.ClusterOperatorStatusCondition {
	var res []configv1.ClusterOperatorStatusCondition
	for _, v := range c.entryMap {
		res = append(res, v)
	}
	sort.SliceStable(res, func(i, j int) bool {
		return string(res[i].Type) < string(res[j].Type)
	})
	return res
}
