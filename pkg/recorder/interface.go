package recorder

import (
	"context"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
)

// Interface that defines the recorder
type Interface interface {
	Record(record.Record) error
}

// FlushInterface extends Recorder by requiring flush
type FlushInterface interface {
	Interface
	Flush(context.Context) error
}

// Driver for the recorder
type Driver interface {
	Save(context.Context, record.MemoryRecords) (record.MemoryRecords, error)
	Prune(context.Context, time.Time) error
}
