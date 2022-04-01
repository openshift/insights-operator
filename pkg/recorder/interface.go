package recorder

import (
	"time"

	"github.com/openshift/insights-operator/pkg/record"
)

// Interface that defines the recorder
type Interface interface {
	Record(record.Record) []error
}

// FlushInterface extends Recorder by requiring flush
type FlushInterface interface {
	Interface
	Flush() error
}

// Driver for the recorder
type Driver interface {
	Save(record.MemoryRecords) (record.MemoryRecords, error)
	Prune(time.Time) error
}
