package conditional

// ConditionWithParams is a type holding a condition with its params
type ConditionWithParams struct {
	Type  ConditionType         `json:"type"`
	Alert *AlertConditionParams `json:"alert,omitempty"`
}

// condition types:

// ConditionType defines conditions to check
type ConditionType string

// AlertIsFiring is a condition to check that alert is firing
// the params are in the field `alert`
const AlertIsFiring ConditionType = "alert_is_firing"

// params:

// AlertConditionParams is a type holding params for alert_is_firing condition
type AlertConditionParams struct {
	// Name of the alert
	Name string `json:"name"`
}
