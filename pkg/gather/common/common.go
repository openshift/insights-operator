package common

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatheringClosure is a struct containing a closure each gatherer returns
// it also contains CanFail field showing if we should just log the failures
type GatheringClosure struct {
	Run     func(context.Context) ([]record.Record, []error)
	CanFail bool
}
