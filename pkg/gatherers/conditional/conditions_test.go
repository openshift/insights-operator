package conditional

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_areAllConditionsSatisfied(t *testing.T) {
	g := &Gatherer{
		firingAlerts:   map[string][]AlertLabels{},
		clusterVersion: "",
	}
	expectedVersion := "4.11.0-0.nightly-2022-05-25-193227"
	conditions := []ConditionWithParams{
		{
			Type:  "alert_is_firing",
			Alert: &AlertConditionParams{Name: "APIRemovedInNextEUSReleaseInUse"},
		},
		{
			Type: "cluster_version_matches",
			ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
				Version: expectedVersion,
			},
		},
	}

	// no conditions are satisfied
	ok, err := g.areAllConditionsSatisfied(conditions)
	assert.NoError(t, err)
	assert.False(t, ok)

	// only one condition is satisfied
	g.firingAlerts["APIRemovedInNextEUSReleaseInUse"] = []AlertLabels{}
	g.clusterVersion = "4.9.0"
	ok, err = g.areAllConditionsSatisfied(conditions)
	assert.NoError(t, err)
	assert.False(t, ok)

	// still one condition
	g.firingAlerts = map[string][]AlertLabels{}
	g.clusterVersion = expectedVersion
	ok, err = g.areAllConditionsSatisfied(conditions)
	assert.NoError(t, err)
	assert.False(t, ok)

	// finally both
	g.firingAlerts["APIRemovedInNextEUSReleaseInUse"] = []AlertLabels{}
	g.clusterVersion = expectedVersion
	ok, err = g.areAllConditionsSatisfied(conditions)
	assert.NoError(t, err)
	assert.True(t, ok)
}
