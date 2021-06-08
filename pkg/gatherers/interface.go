// gatherers package contains interfaces to be implemented by gatherers and subpackages with actual gatherers
package gatherers

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"
)

// Interface is an interface for gathering types
type Interface interface {
	// GetName returns the name of the gatherer
	GetName() string

	// GetGatheringFunctions returns all the gathering function implemented by current gatherer
	GetGatheringFunctions() map[string]GatheringClosure
}

// CustomPeriodGatherer gatherers implementing this interface may not get to each archive
// and their period can be different from interval in the config(equal or higher, but never lower)
type CustomPeriodGatherer interface {
	Interface

	// ShouldBeProcessedNow returns true when it's time to process the gatherer
	// gatherer is responsible of tracking the time itself
	ShouldBeProcessedNow() bool
	// UpdateLastProcessingTime is called when the gatherer is about to be processed,
	// so that it can update its last processed time for example.
	UpdateLastProcessingTime()
}

// GatheringClosure is a struct containing a closure each gatherer returns
// it also contains CanFail field showing if we should just log the failures
type GatheringClosure struct {
	Run     func(context.Context) ([]record.Record, []error)
	CanFail bool
}
