package conditional

// To create a new condition follow the next steps:
// 1. Add *ConditionParams field to ConditionWithParams
// 2. Create a value in ConditionType enum
// 3. Create *ConditionParam type
// 4. Modify areAllConditionsSatisfied function to handle the new condition. All the initialization code such as
// populating a cache with values should be written in conditional_gatherer.go file
// 5. Add validation in gathering_rule.schema.json

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// ConditionWithParams is a type holding a condition with its params
type ConditionWithParams struct {
	Type                  ConditionType                         `json:"type"`
	Alert                 *AlertConditionParams                 `json:"alert,omitempty"`
	ClusterVersionMatches *ClusterVersionMatchesConditionParams `json:"cluster_version_matches,omitempty"`
}

// condition types:

// ConditionType defines conditions to check
type ConditionType string

// AlertIsFiring is a condition to check that alert is firing
// the params are in the field `alert`
const AlertIsFiring ConditionType = "alert_is_firing"

// ClusterVersionMatches is a condition to check that the current cluster version
// matches the provided semantic versioning expression
const ClusterVersionMatches ConditionType = "cluster_version_matches"

// params:

// AlertConditionParams is a type holding params for alert_is_firing condition
type AlertConditionParams struct {
	// Name of the alert
	Name string `json:"name"`
}

// ClusterVersionMatchesConditionParams is a type holding params for cluster_version_matches condition
type ClusterVersionMatchesConditionParams struct {
	// Version is a semantic versioning expression
	Version string `json:"version"`
}

// conditions definitions:

// areAllConditionsSatisfied returns true if all the conditions are satisfied, for example if the condition is
// to check if a metric is firing, it will look at that metric and return the result according to that
func (g *Gatherer) areAllConditionsSatisfied(conditions []ConditionWithParams) (bool, error) {
	for _, condition := range conditions {
		switch condition.Type {
		case AlertIsFiring:
			if condition.Alert == nil {
				return false, fmt.Errorf("alert field should not be nil")
			}

			if firing, err := g.isAlertFiring(condition.Alert.Name); !firing || err != nil {
				return false, err
			}
		case ClusterVersionMatches:
			if condition.ClusterVersionMatches == nil {
				return false, fmt.Errorf("cluster_version_matches field should not be nil")
			}

			if doesMatch, err := g.doesClusterVersionMatch(condition.ClusterVersionMatches.Version); !doesMatch || err != nil {
				return false, err
			}
		default:
			return false, fmt.Errorf("unknown condition type: %v", condition.Type)
		}
	}

	return true, nil
}

// isAlertFiring using the cache it returns true if the alert is firing
func (g *Gatherer) isAlertFiring(alertName string) (bool, error) {
	if g.firingAlerts == nil {
		return false, fmt.Errorf("alerts cache is missing")
	}

	_, alertIsFiring := g.firingAlerts[alertName]
	return alertIsFiring, nil
}

// doesClusterVersionMatch checks if current cluster version matches the provided expression and returns true if so
func (g *Gatherer) doesClusterVersionMatch(expectedVersionExpression string) (bool, error) {
	if len(g.clusterVersion) == 0 {
		return false, fmt.Errorf("cluster version is missing")
	}

	clusterVersion, err := semver.Parse(g.clusterVersion)
	if err != nil {
		return false, err
	}

	expectedRange, err := semver.ParseRange(expectedVersionExpression)
	if err != nil {
		return false, err
	}

	// ignore everything after the first three numbers
	clusterVersion.Pre = nil
	clusterVersion.Build = nil

	return expectedRange(clusterVersion), nil
}
