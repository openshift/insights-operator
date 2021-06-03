package periodic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/recorder"
)

func Test_Controller_CustomPeriodGatherer(t *testing.T) {
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{CanFail: true},
		&gather.MockCustomPeriodGatherer{Period: 999 * time.Hour},
	}, 1*time.Hour)

	c.Gather()
	// 6 gatherers + metadata
	assert.Len(t, mockRecorder.Records, 7)
	mockRecorder.Reset()

	c.Gather()
	// 5 gatherers + metadata (one is low priority and we need to wait 999 hours to get it
	assert.Len(t, mockRecorder.Records, 6)
	mockRecorder.Reset()
}

func Test_Controller_CustomPeriodGathererNoPeriod(t *testing.T) {
	mockGatherer := gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true}
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{CanFail: true},
		&mockGatherer,
	}, 1*time.Hour)

	c.Gather()
	// 6 gatherers + metadata
	assert.Len(t, mockRecorder.Records, 7)
	assert.Equal(t, 1, mockGatherer.ShouldBeProcessedNowWasCalledNTimes)
	assert.Equal(t, 1, mockGatherer.UpdateLastProcessingTimeWasCalledNTimes)
	mockRecorder.Reset()

	mockGatherer.ShouldBeProcessed = false

	c.Gather()
	// 5 gatherers + metadata (we've just disabled one gatherer)
	assert.Len(t, mockRecorder.Records, 6)
	assert.Equal(t, 2, mockGatherer.ShouldBeProcessedNowWasCalledNTimes)
	// ShouldBeProcessedNow had returned false so we didn't call UpdateLastProcessingTime
	assert.Equal(t, 1, mockGatherer.UpdateLastProcessingTimeWasCalledNTimes)
	mockRecorder.Reset()

	mockGatherer.ShouldBeProcessed = true

	c.Gather()
	assert.Len(t, mockRecorder.Records, 7)
	assert.Equal(t, 3, mockGatherer.ShouldBeProcessedNowWasCalledNTimes)
	assert.Equal(t, 2, mockGatherer.UpdateLastProcessingTimeWasCalledNTimes)
	mockRecorder.Reset()
}

// Test_Controller_FailingGatherer tests that metadata file doesn't grow with failing gatherer functions
func Test_Controller_FailingGatherer(t *testing.T) {
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockFailingGatherer{},
	}, 3*time.Second)

	c.Gather()
	metadataFound := false
	// failing gatherer failed 5x (see GatherFailuresCountThreshold const) + metadata
	assert.Len(t, mockRecorder.Records, 6)
	for i := range mockRecorder.Records {
		// find metadata record
		if mockRecorder.Records[i].Name != recorder.MetadataRecordName {
			continue
		}
		metadataFound = true
		b, err := mockRecorder.Records[i].Item.Marshal(context.Background())
		assert.NoError(t, err)
		metaData := make(map[string]interface{})
		err = json.Unmarshal(b, &metaData)
		assert.NoError(t, err)
		assert.Len(t, metaData["status_reports"].([]interface{}), 1,
			fmt.Sprintf("Only one function for %s expected ", c.gatherers[0].GetName()))
	}
	assert.Truef(t, metadataFound, fmt.Sprintf("%s not found in records", recorder.MetadataRecordName))
	mockRecorder.Reset()
}

// TODO: cover more things

func getMocksForPeriodicTest(listGatherers []gatherers.Interface, interval time.Duration) (*Controller, *recorder.MockRecorder) {
	mockConfigurator := config.MockConfigurator{Conf: &config.Controller{
		Report:   true,
		Interval: interval,
		Gather:   []string{gather.AllGatherersConst},
	}}
	mockRecorder := recorder.MockRecorder{}

	return New(&mockConfigurator, &mockRecorder, listGatherers, nil), &mockRecorder
}
