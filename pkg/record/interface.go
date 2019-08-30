package record

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Interface interface {
	Record(Record) error
}

type FlushInterface interface {
	Interface
	Flush(context.Context) error
}

type Record struct {
	Name     string
	Captured time.Time

	Fingerprint string
	Item        Marshalable
}

type Marshalable interface {
	Marshal(context.Context) ([]byte, error)
}

type JSONMarshaller struct {
	Object interface{}
}

func (m JSONMarshaller) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Object)
}

// ErrSkipRecord instructs Collect to ignore the provided recorder.
var ErrSkipRecord = fmt.Errorf("skip recording")

// Aggregate allows multiple array record gatherers to be passed to Collect.
func Aggregate(fns ...func() ([]Record, []error)) func() ([]Record, []error) {
	return func() ([]Record, []error) {
		var records []Record
		var errs []error
		for _, fn := range fns {
			r, e := fn()
			records = append(records, r...)
			errs = append(errs, e...)
		}
		return records, errs
	}
}

// Collect is a helper for gathering a large set of records from generic functions.
func Collect(ctx context.Context, recorder Interface, bulkFn func() ([]Record, []error), fns ...func() (Record, error)) error {
	var errors []string
	if bulkFn != nil {
		records, errs := bulkFn()
		for _, err := range errs {
			errors = append(errors, err.Error())
		}
		for _, record := range records {
			if err := recorder.Record(record); err != nil {
				errors = append(errors, fmt.Sprintf("unable to record %s: %v", record.Name, err.Error()))
				continue
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, fn := range fns {
		record, err := fn()
		if err != nil {
			if err != ErrSkipRecord {
				errors = append(errors, err.Error())
			}
			continue
		}
		if err := recorder.Record(record); err != nil {
			errors = append(errors, fmt.Sprintf("unable to record %s: %v", record.Name, err.Error()))
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if len(errors) > 0 {
		sort.Strings(errors)
		errors = uniqueStrings(errors)
		return fmt.Errorf("failed to gather cluster infrastructure: %s", strings.Join(errors, ", "))
	}
	return nil
}

func uniqueStrings(arr []string) []string {
	var last int
	for i := 1; i < len(arr); i++ {
		if arr[i] == arr[last] {
			continue
		}
		last++
		if last != i {
			arr[last] = arr[i]
		}
	}
	if last < len(arr) {
		last++
	}
	return arr[:last]
}
