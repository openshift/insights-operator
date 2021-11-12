package conditional

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
