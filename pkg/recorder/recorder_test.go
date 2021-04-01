package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/tests"
)

// RawReport implements Marshable interface
type RawReport struct{ Data string }

// Marshal returns raw bytes
func (r RawReport) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.Data), nil
}

// GetExtension returns extension for raw marshaller
func (r RawReport) GetExtension() string {
	return ""
}

// RawInvalidReport implements Marshable interface but throws an error
type RawInvalidReport struct{}

// Marshal returns raw bytes
func (r RawInvalidReport) Marshal(_ context.Context) ([]byte, error) {
	return nil, &json.UnsupportedTypeError{}
}

// GetExtension returns extension for raw marshaller
func (r RawInvalidReport) GetExtension() string {
	return ""
}

type driverMock struct {
	mock.Mock
}

func (d *driverMock) Save(records record.MemoryRecords) (record.MemoryRecords, error) {
	args := d.Called()
	return records, args.Error(1)
}

func (d *driverMock) Prune(olderThan time.Time) error {
	args := d.Called()
	return args.Error(1)
}

func newRecorder() Recorder {
	driver := driverMock{}
	driver.On("Save").Return(nil, nil)

	anonymizer, _ := anonymization.NewAnonymizer(nil, "", nil)

	interval, _ := time.ParseDuration("1m")
	return Recorder{
		driver:     &driver,
		interval:   interval,
		maxAge:     interval * 6 * 24,
		records:    make(map[string]*record.MemoryRecord),
		flushCh:    make(chan struct{}, 1),
		flushSize:  8 * 1024 * 1024,
		anonymizer: anonymizer,
	}
}

func Test_Record(t *testing.T) {
	rec := newRecorder()
	err := rec.Record(record.Record{
		Name: "config/mock1",
		Item: RawReport{Data: "mock1"},
	})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(rec.records))
}

func Test_Record_Duplicated(t *testing.T) {
	rec := newRecorder()
	_ = rec.Record(record.Record{
		Name:        "config/mock1",
		Item:        RawReport{Data: "mock1"},
		Fingerprint: "abc",
	})
	err := rec.Record(record.Record{
		Name:        "config/mock1",
		Item:        RawReport{Data: "mock1"},
		Fingerprint: "abc",
	})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(rec.records))
}

func Test_Record_CantBeSerialized(t *testing.T) {
	rec := newRecorder()
	err := rec.Record(record.Record{
		Name: "config/mock1",
		Item: RawInvalidReport{},
	})
	assert.Error(t, err)
}

func Test_Record_Flush(t *testing.T) {
	rec := newRecorder()
	for i := range []int{1, 2, 3} {
		_ = rec.Record(record.Record{
			Name: fmt.Sprintf("config/mock%d", i),
			Item: RawReport{Data: "mockdata"},
		})
	}
	err := rec.Flush()
	assert.Nil(t, err)
	assert.Equal(t, int64(0), rec.size)
}

func Test_Record_FlushEmptyRecorder(t *testing.T) {
	rec := newRecorder()
	err := rec.Flush()
	assert.Nil(t, err)
}
