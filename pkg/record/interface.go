package record

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"k8s.io/klog"
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
	GetExtension() string
}

type JSONMarshaller struct {
	Object interface{}
}

func (m JSONMarshaller) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Object)
}

// GetExtension return extension for json marshaller
func (m JSONMarshaller) GetExtension() string {
	return "json"
}

type gatherMetadata struct {
	StatusReports []gatherStatusReport `json:"status_reports"`
	MemoryAlloc   uint64               `json:"memory_alloc_bytes"`
	Uptime        float64              `json:"uptime_seconds"`
}

type gatherStatusReport struct {
	Name         string        `json:"name"`
	Duration     time.Duration `json:"duration_in_ms"`
	RecordsCount int           `json:"records_count"`
	Errors       []string      `json:"errors"`
}

var startTime time.Time

// Collect is a helper for gathering a large set of records from generic functions.
func Collect(ctx context.Context, recorder Interface, bulkFns ...func() ([]Record, []error)) error {
	var errors []string
	var statusReports []gatherStatusReport

	if startTime.IsZero() {
		startTime = time.Now()
	}

	for _, bulkFn := range bulkFns {
		gatherName := runtime.FuncForPC(reflect.ValueOf(bulkFn).Pointer()).Name()
		klog.V(5).Infof("Gathering %s", gatherName)
		start := time.Now()
		records, errs := bulkFn()

		shortName := strings.Replace(gatherName, "github.com/openshift/insights-operator/pkg/gather/", "", 1)
		shortName = strings.Replace(shortName, ".func1", "", 1)
		elapsed := time.Since(start).Truncate(time.Millisecond)
		statusReport := gatherStatusReport{shortName, time.Duration(elapsed.Milliseconds()), len(records), extractErrors(errs)}
		statusReports = append(statusReports, statusReport)
		for _, err := range errs {
			errors = append(errors, err.Error())
		}
		for _, record := range records {
			if err := recorder.Record(record); err != nil {
				errors = append(errors, fmt.Sprintf("unable to record %s: %v", record.Name, err))
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	// Creates the gathering performance report
	if err := recordGatherReport(recorder, statusReports); err != nil {
		errors = append(errors, fmt.Sprintf("unable to record io status reports: %v", err))
	}
	if len(errors) > 0 {
		sort.Strings(errors)
		errors = uniqueStrings(errors)
		return fmt.Errorf("%s", strings.Join(errors, ", "))
	}
	return nil
}

func recordGatherReport(recorder Interface, report []gatherStatusReport) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metadata := gatherMetadata{report, m.HeapAlloc, time.Since(startTime).Truncate(time.Millisecond).Seconds()}
	r := Record{Name: "insights-operator/gathers", Item: JSONMarshaller{Object: metadata}}
	return recorder.Record(r)
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

func extractErrors(errors []error) []string {
	var errStrings []string
	for _, err := range errors {
		errStrings = append(errStrings, err.Error())
	}
	return errStrings
}
