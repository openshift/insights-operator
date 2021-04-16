package diskrecorder

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
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
	return DiskRecorder{basePath: "/tmp"}
}

func Test_Diskrecorder_Save(t *testing.T) {
	dr := newDiskRecorder()
	records := getMemoryRecords()
	saved, err := dr.Save(records)
	assert.Nil(t, err)
	assert.Len(t, saved, len(records))
}

func Test_Diskrecorder_SaveInvalidPath(t *testing.T) {
	dr := DiskRecorder{basePath: "/tmp/this-path-not-exists"}
	records := getMemoryRecords()
	saved, err := dr.Save(records)
	assert.Error(t, err)
	assert.Nil(t, saved)
}

func Test_Diskrecorder_SaveFailsIfDuplicatedReport(t *testing.T) {
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
	_, _ = dr.Save(records)
	saved, err := dr.Save(records)
	assert.Error(t, err)
	assert.Nil(t, saved)
}

func Test_Diskrecorder_Summary(t *testing.T) {
	since := time.Now().Add(time.Duration(-5) * time.Minute)
	dr := newDiskRecorder()
	reader, ok, err := dr.Summary(context.TODO(), since)
	assert.IsType(t, reader, reader)
	assert.True(t, ok)
	assert.Nil(t, err)
}

func Test_Diskrecorder_Prune(t *testing.T) {
	olderThan := time.Now().Add(time.Duration(5) * time.Minute)
	dr := newDiskRecorder()
	err := dr.Prune(olderThan)
	assert.Nil(t, err)
}
