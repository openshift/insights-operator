package conditional

import (
	"github.com/openshift/insights-operator/pkg/gatherers"
)

// GatheringRule is a rule consisting of conditions and gathering functions to run if all conditions are met.
type GatheringRule struct {
	// dive means it will go inside the slice and check all the values
	// conditions can be empty
	Conditions []ConditionWithParams `json:"conditions" validate:"dive"`
	// gathering functions can't be empty
	GatheringFunctions GatheringFunctions `json:"gathering_functions" validate:"dive"`
}

// GathererFunctionBuilderPtr defines a pointer to a gatherer function builder
type GathererFunctionBuilderPtr = func(*Gatherer, interface{}) (gatherers.GatheringClosure, error)
