package conditional

import "github.com/openshift/insights-operator/pkg/gatherers"

// conditionType defines conditions to check
type conditionType string

// alertIsFiring is a condition to check that alert is firing.
// Params:
//   - name - name of the alert
const alertIsFiring conditionType = "alert_is_firing"

// conditionWithParams is a type holding a condition with its params
type conditionWithParams struct {
	Type   conditionType          `json:"type"`
	Params map[string]interface{} `json:"params"`
}

// GatheringFunctionParams is a type to store gathering function parameters
type GatheringFunctionParams map[string]interface{}

// gatheringFunctionName defines functions of conditional gatherer
type gatheringFunctionName string

// gatherLogsOfNamespace is a function collecting logs of the provided namespace. See file gather_logs_of_namespace.go
const gatherLogsOfNamespace gatheringFunctionName = "logs_of_namespace"

// gatherImageStreamsOfNamespace is a function collecting image streams of the provided namespace.
// See file gather_logs_of_namespace.go
const gatherImageStreamsOfNamespace gatheringFunctionName = "image_streams_of_namespace"

// gatheringRule is a rule consisting of conditions and gathering functions to run if all conditions are met.
type gatheringRule struct {
	Conditions         []conditionWithParams                             `json:"conditions"`
	GatheringFunctions map[gatheringFunctionName]GatheringFunctionParams `json:"gathering_functions"`
}

type gathererFunctionBuilderPtr = func(*Gatherer, GatheringFunctionParams) (gatherers.GatheringClosure, error)
