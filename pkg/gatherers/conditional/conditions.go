package conditional

import (
	"encoding/json"
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

// AlertIsFiring is a condition to check that alert is firing
const AlertIsFiring ConditionType = "alert_is_firing"

// NewParams creates an instance of params type for this condition type
func (ct ConditionType) NewParams(jsonParam []byte) (interface{}, error) {
	switch ct { //nolint:gocritic
	case AlertIsFiring:
		var result AlertIsFiringConditionParams
		err := json.Unmarshal(jsonParam, &result)
		return result, err
	}
	return nil, fmt.Errorf("unable to create params for %T: %v", ct, ct)
}

// params:

// AlertIsFiringConditionParams is a type holding params for alert_is_firing condition
type AlertIsFiringConditionParams struct {
	// Name of the alert
	Name string `json:"name"`
}
