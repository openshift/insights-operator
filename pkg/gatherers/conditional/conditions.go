package conditional

import (
	"fmt"
)

// ConditionWithParams is a type holding a condition with its params
type ConditionWithParams struct {
	Type   ConditionType `json:"type"`
	Params interface{}   `json:"params"`
}

// condition types:

// ConditionType defines conditions to check
type ConditionType string

// AlertIsFiring is a condition to check that alert is firing.
const AlertIsFiring ConditionType = "alert_is_firing"

// IsValid checks if the value is one of allowed for this "enum"
func (ct ConditionType) IsValid() error {
	switch ct {
	case AlertIsFiring:
		return nil
	}
	return fmt.Errorf("invalid value for %T", ct)
}

// params:

// AlertIsFiringConditionParams is a type holding params for alert_is_firing condition
type AlertIsFiringConditionParams struct {
	// Name of the alert. Only strings with length from 1 to 128 (including) containing alphanumeric characters are valid
	Name string `json:"name" validate:"min=1,max=128,alphanum"`
}

// ConditionTypeToParamsType maps ConditionType to Params, needed for validation,
// you gotta add a new value here whenever you implement a new condition
var ConditionTypeToParamsType = map[ConditionType]interface{}{
	AlertIsFiring: AlertIsFiringConditionParams{},
}
