package diskrecorder

import (
	"context"
	"fmt"
	"os"
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

func newDiskRecorder() (DiskRecorder, error) {
	basePath := "/tmp"
	path, err := os.MkdirTemp(basePath, "insights-operator")
	return DiskRecorder{basePath: path}, err
}

func Test_Diskrecorder_Save(t *testing.T) {
	dr, err := newDiskRecorder()
	assert.NoError(t, err)
	records := getMemoryRecords()
	saved, err := dr.Save(records)
	assert.NoError(t, err)
	assert.Len(t, saved, len(records))
	assert.WithinDuration(t, time.Now(), dr.lastRecording, 10*time.Second)

	err = removePath(dr)
	assert.NoError(t, err)
}

func Test_Diskrecorder_SaveInvalidPath(t *testing.T) {
	dr := DiskRecorder{basePath: "/tmp/this-path-not-exists"}
	records := getMemoryRecords()
	saved, err := dr.Save(records)
	assert.Error(t, err)
	assert.Nil(t, saved)

	err = removePath(dr)
	assert.NoError(t, err)
}

func Test_Diskrecorder_SaveFailsIfDuplicatedReport(t *testing.T) {
	dr, err := newDiskRecorder()
	assert.NoError(t, err)
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

	err = removePath(dr)
	assert.NoError(t, err)
}

func Test_Diskrecorder_Summary(t *testing.T) {
	since := time.Now().Add(time.Duration(-2) * time.Second)
	dr, err := newDiskRecorder()
	assert.NoError(t, err)

	records := getMemoryRecords()
	// we need some archives in the filesystem for the Summmary method
	_, err = dr.Save(records)
	assert.NoError(t, err)

	source, ok, err := dr.Summary(context.Background(), since)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, source)

	err = removePath(dr)
	assert.NoError(t, err)
}

func Test_Diskrecorder_Prune(t *testing.T) {
	olderThan := time.Now().Add(time.Duration(5) * time.Minute)
	dr, err := newDiskRecorder()
	assert.NoError(t, err)
	err = dr.Prune(olderThan)
	assert.NoError(t, err)

	err = removePath(dr)
	assert.NoError(t, err)
}

func removePath(d DiskRecorder) error {
	return os.RemoveAll(d.basePath)
}
