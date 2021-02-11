package recorder

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type driverMock struct {
	mock.Mock
}

func (d *driverMock) Save(ctx context.Context, records record.MemoryRecords) (record.MemoryRecords, error) {
	args := d.Called()
	return records, args.Error(1)
}

func (d *driverMock) Prune(ctx context.Context, olderThan time.Time) error {
	args := d.Called()
	return args.Error(1)
}

func newRecorder() Recorder {
	driver := driverMock{}
	driver.On("Save").Return(nil, nil)

	interval, _ := time.ParseDuration("1m")
	return Recorder{
		driver:    &driver,
		interval:  interval,
		maxAge:    interval * 6 * 24,
		records:   make(map[string]*record.MemoryRecord),
		flushCh:   make(chan struct{}, 1),
		flushSize: 8 * 1024 * 1024,
	}
}

func TestRecord(t *testing.T) {
	rec := newRecorder()
	err := rec.Record(record.Record{
		Name: "config/mock1",
		Item: tests.RawReport{Data: "mock1"},
	})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(rec.records))
}

func TestRecordDuplicated(t *testing.T) {
	rec := newRecorder()
	_ = rec.Record(record.Record{
		Name:        "config/mock1",
		Item:        tests.RawReport{Data: "mock1"},
		Fingerprint: "abc",
	})
	err := rec.Record(record.Record{
		Name:        "config/mock1",
		Item:        tests.RawReport{Data: "mock1"},
		Fingerprint: "abc",
	})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(rec.records))
}

func TestRecordCantBeSerialized(t *testing.T) {
	rec := newRecorder()
	err := rec.Record(record.Record{
		Name: "config/mock1",
		Item: tests.RawInvalidReport{},
	})
	assert.Error(t, err)
}

func TestFlush(t *testing.T) {
	rec := newRecorder()
	for i := range []int{1, 2, 3} {
		_ = rec.Record(record.Record{
			Name: fmt.Sprintf("config/mock%d", i),
			Item: tests.RawReport{Data: "mockdata"},
		})
	}
	err := rec.Flush(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, int64(0), rec.size)
}

func TestFlushEmptyRecorder(t *testing.T) {
	rec := newRecorder()
	err := rec.Flush(context.TODO())
	assert.Nil(t, err)
}
