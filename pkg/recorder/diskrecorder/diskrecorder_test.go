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
	assert.NoError(t, err)
	assert.Len(t, saved, len(records))
	assert.WithinDuration(t, time.Now(), dr.lastRecording, 10*time.Second)
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
	source, ok, err := dr.Summary(context.TODO(), since)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, source)
}

func Test_Diskrecorder_Prune(t *testing.T) {
	olderThan := time.Now().Add(time.Duration(5) * time.Minute)
	dr := newDiskRecorder()
	err := dr.Prune(olderThan)
	assert.NoError(t, err)
}
