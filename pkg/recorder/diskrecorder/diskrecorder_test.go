package diskrecorder

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/tests"
	"github.com/stretchr/testify/assert"
)

func getMemoryRecords() record.MemoryRecords {
	var records record.MemoryRecords
	for i := range []int{1, 2, 3} {
		records = append(records, record.MemoryRecord{
			Name: fmt.Sprintf("config/mock%d", i),
			At:   time.Now(),
			Data: []byte("data"),
		})
	}
	return records
}

func newDiskRecorder() DiskRecorder {
	return DiskRecorder{basePath: tests.BaseTestPath}
}

func TestSave(t *testing.T) {
	dr := newDiskRecorder()
	records := getMemoryRecords()
	saved, err := dr.Save(context.TODO(), records)
	assert.Nil(t, err)
	assert.Len(t, saved, len(records))
}

func TestSaveInvalidPath(t *testing.T) {
	dr := DiskRecorder{basePath: "/tmp/this-path-not-exists"}
	records := getMemoryRecords()
	saved, err := dr.Save(context.TODO(), records)
	assert.Error(t, err)
	assert.Nil(t, saved)
}

func TestSaveFailsIfDuplicatedReport(t *testing.T) {
	dr := newDiskRecorder()
	records := record.MemoryRecords{
		record.MemoryRecord{
			Name: "config/mock1",
			Data: []byte("data"),
		},
		record.MemoryRecord{
			Name: "config/mock2",
			Data: []byte("data"),
		},
	}
	_, _ = dr.Save(context.TODO(), records)
	saved, err := dr.Save(context.TODO(), records)
	assert.Error(t, err)
	assert.Nil(t, saved)
}

func TestSummary(t *testing.T) {
	since := time.Now().Add(time.Duration(-5) * time.Minute)
	dr := newDiskRecorder()
	reader, ok, err := dr.Summary(context.TODO(), since)
	// assert.Implements(t, io.ReadCloser{}, reader)
	assert.IsType(t, (io.ReadCloser)(reader), reader)
	assert.True(t, ok)
	assert.Nil(t, err)
}

func TestPrune(t *testing.T) {
	olderThan := time.Now().Add(time.Duration(5) * time.Minute)
	dr := newDiskRecorder()
	err := dr.Prune(context.TODO(), olderThan)
	assert.Nil(t, err)
}
