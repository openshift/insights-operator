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
)

var (
	mock1Name = "config/mock1"
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

func (d *driverMock) Prune(time.Time) error {
	args := d.Called()
	return args.Error(1)
}

func newRecorder(maxArchiveSize int64) Recorder {
	driver := driverMock{}
	driver.On("Save").Return(nil, nil)

	anonymizer, _ := anonymization.NewAnonymizer("", nil, nil)

	interval, _ := time.ParseDuration("1m")
	return Recorder{
		driver:         &driver,
		interval:       interval,
		maxAge:         interval * 6 * 24,
		maxArchiveSize: maxArchiveSize,
		records:        make(map[string]*record.MemoryRecord),
		anonymizer:     anonymizer,
	}
}

func Test_Record(t *testing.T) {
	rec := newRecorder(MaxArchiveSize)
	err := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawReport{Data: "mock1"},
	})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(rec.records))
}

func Test_Record_Duplicated(t *testing.T) {
	rec := newRecorder(MaxArchiveSize)
	_ = rec.Record(record.Record{
		Name:        mock1Name,
		Item:        RawReport{Data: "mock1"},
		Fingerprint: "abc",
	})
	err := rec.Record(record.Record{
		Name:        mock1Name,
		Item:        RawReport{Data: "mock1"},
		Fingerprint: "abc",
	})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(rec.records))
}

func Test_Record_CantBeSerialized(t *testing.T) {
	rec := newRecorder(MaxArchiveSize)
	err := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawInvalidReport{},
	})
	assert.Error(t, err)
}

func Test_Record_Flush(t *testing.T) {
	rec := newRecorder(MaxArchiveSize)
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
	rec := newRecorder(MaxArchiveSize)
	err := rec.Flush()
	assert.Nil(t, err)
}

func Test_Record_ArchiveSizeExceeded(t *testing.T) {
	data := "data bigger than 4 bytes"
	maxArchiveSize := int64(4)
	rec := newRecorder(maxArchiveSize)
	err := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawReport{
			Data: data,
		},
	})
	assert.Equal(
		t,
		err,
		fmt.Errorf(
			"record %s(size=%d) exceeds the archive size limit %d and will not be included in the archive",
			mock1Name,
			len([]byte(data)),
			maxArchiveSize))
}
