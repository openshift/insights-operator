package conditional

import (
	"github.com/openshift/insights-operator/pkg/gatherers"
)

// GatheringRule is a rule consisting of conditions and gathering functions to run if all conditions are met,
// gathering_rule.schema.json describes valid values for this struct
type GatheringRule struct {
	// conditions can be empty
	Conditions []ConditionWithParams `json:"conditions"`
	// gathering functions can't be empty
	GatheringFunctions GatheringFunctions `json:"gathering_functions"`
}

// GathererFunctionBuilderPtr defines a pointer to a gatherer function builder
type GathererFunctionBuilderPtr = func(*Gatherer, interface{}) (gatherers.GatheringClosure, error)

// Alert defines basic alert attributes (basically alert labels)
type Alert struct {
	Name   string
	Labels map[string]string
}
