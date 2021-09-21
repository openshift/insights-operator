package status

import (
	v1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// OperatorDisabled defines the condition type when the operator's primary function has been disabled
	OperatorDisabled v1.ClusterStatusConditionType = "Disabled"
	// InsightsUploadDegraded defines the condition type (when set to True) when an archive can't be successfully uploaded
	InsightsUploadDegraded v1.ClusterStatusConditionType = "UploadDegraded"
	// InsightsDownloadDegraded defines the condition type (when set to True) when the Insights report can't be successfully downloaded
	InsightsDownloadDegraded v1.ClusterStatusConditionType = "InsightsDownloadDegraded"
)

type conditionsMap map[v1.ClusterStatusConditionType]v1.ClusterOperatorStatusCondition

type conditions struct {
	entryMap conditionsMap
}

func newConditions(cos *v1.ClusterOperatorStatus) *conditions {
	entries := conditionsMap{}
	for _, c := range cos.Conditions {
		entries[c.Type] = c
	}
	return &conditions{
		entryMap: entries,
	}
}

func (c *conditions) setCondition(condition v1.ClusterStatusConditionType,
	status v1.ConditionStatus, message, reason string, lastTime metav1.Time) {
	entries := make(conditionsMap)
	for k, v := range c.entryMap {
		entries[k] = v
	}

	existing, ok := c.entryMap[condition]
	if !ok || existing.Status != status || existing.Reason != reason {
		if lastTime.IsZero() {
			lastTime = metav1.Now()
		}
		entries[condition] = v1.ClusterOperatorStatusCondition{
			Type:               condition,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: lastTime,
		}
	}

	c.entryMap = entries
}

func (c *conditions) removeCondition(condition v1.ClusterStatusConditionType) {
	if !c.hasCondition(condition) {
		return
	}

	entries := make(conditionsMap)
	for k, v := range c.entryMap {
		if k == condition {
			continue
		}
		entries[k] = v
	}

	c.entryMap = entries
}

func (c *conditions) hasCondition(condition v1.ClusterStatusConditionType) bool {
	_, ok := c.entryMap[condition]
	return ok
}

func (c *conditions) findCondition(condition v1.ClusterStatusConditionType) *v1.ClusterOperatorStatusCondition {
	existing, ok := c.entryMap[condition]
	if ok {
		return &existing
	}
	return nil
}

func (c *conditions) entries() []v1.ClusterOperatorStatusCondition {
	var res []v1.ClusterOperatorStatusCondition
	for _, v := range c.entryMap {
		res = append(res, v)
	}
	return res
}
