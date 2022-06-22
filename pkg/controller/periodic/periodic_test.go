package periodic

import (
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
		&gather.MockGatherer{},
		&gather.MockCustomPeriodGatherer{Period: 999 * time.Hour},
	}, 1*time.Hour)

	c.Gather()
	// 6 gatherers + metadata
	assert.Len(t, mockRecorder.Records, 7)
	mockRecorder.Clear()

	c.Gather()
	// 5 gatherers + metadata (one is low priority and we need to wait 999 hours to get it
	assert.Len(t, mockRecorder.Records, 6)
	mockRecorder.Clear()
}

func Test_Controller_Run(t *testing.T) {
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{},
	}, 1*time.Hour)

	// No delay, 5 gatherers + metadata
	stopCh := make(chan struct{})
	go c.Run(stopCh, 0)
	time.Sleep(100 * time.Millisecond)
	stopCh <- struct{}{}
	assert.Len(t, mockRecorder.Records, 6)
	mockRecorder.Clear()

	// 2 sec delay, 5 gatherers + metadata
	stopCh = make(chan struct{})
	go c.Run(stopCh, 2*time.Second)
	time.Sleep(2 * time.Second)
	stopCh <- struct{}{}
	assert.Len(t, mockRecorder.Records, 6)
	mockRecorder.Clear()

	// 2 hour delay, stop before delay ends
	stopCh = make(chan struct{})
	go c.Run(stopCh, 2*time.Hour)
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, mockRecorder.Records, 0)
	stopCh <- struct{}{}
	assert.Len(t, mockRecorder.Records, 0)
	mockRecorder.Clear()
}

func Test_Controller_periodicTrigger(t *testing.T) {
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{},
	}, 1*time.Hour)

	// 1 sec interval, 5 gatherers + metadata
	c.configurator.Config().Interval = 1 * time.Second
	stopCh := make(chan struct{})
	go c.periodicTrigger(stopCh)
	// 2 intervals
	time.Sleep(2200 * time.Millisecond)
	stopCh <- struct{}{}
	assert.Len(t, mockRecorder.Records, 12)
	mockRecorder.Clear()

	// 2 hour interval, stop before delay ends
	c.configurator.Config().Interval = 2 * time.Hour
	stopCh = make(chan struct{})
	go c.periodicTrigger(stopCh)
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, mockRecorder.Records, 0)
	stopCh <- struct{}{}
	assert.Len(t, mockRecorder.Records, 0)
	mockRecorder.Clear()
}

func Test_Controller_Sources(t *testing.T) {
	mockGatherer := gather.MockGatherer{}
	mockCustomPeriodGatherer := gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true}
	// 1 Gatherer ==> 1 source
	c, _ := getMocksForPeriodicTest([]gatherers.Interface{
		&mockGatherer,
	}, 1*time.Hour)
	assert.Len(t, c.Sources(), 1)

	// 2 Gatherer ==> 2 source
	c, _ = getMocksForPeriodicTest([]gatherers.Interface{
		&mockGatherer,
		&mockCustomPeriodGatherer,
	}, 1*time.Hour)
	assert.Len(t, c.Sources(), 2)
}

func Test_Controller_CustomPeriodGathererNoPeriod(t *testing.T) {
	mockGatherer := gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true}
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{},
		&mockGatherer,
	}, 1*time.Hour)

	c.Gather()
	// 6 gatherers + metadata
	assert.Len(t, mockRecorder.Records, 7)
	assert.Equal(t, 1, mockGatherer.ShouldBeProcessedNowWasCalledNTimes)
	assert.Equal(t, 1, mockGatherer.UpdateLastProcessingTimeWasCalledNTimes)
	mockRecorder.Clear()

	mockGatherer.ShouldBeProcessed = false

	c.Gather()
	// 5 gatherers + metadata (we've just disabled one gatherer)
	assert.Len(t, mockRecorder.Records, 6)
	assert.Equal(t, 2, mockGatherer.ShouldBeProcessedNowWasCalledNTimes)
	// ShouldBeProcessedNow had returned false so we didn't call UpdateLastProcessingTime
	assert.Equal(t, 1, mockGatherer.UpdateLastProcessingTimeWasCalledNTimes)
	mockRecorder.Clear()

	mockGatherer.ShouldBeProcessed = true

	c.Gather()
	assert.Len(t, mockRecorder.Records, 7)
	assert.Equal(t, 3, mockGatherer.ShouldBeProcessedNowWasCalledNTimes)
	assert.Equal(t, 2, mockGatherer.UpdateLastProcessingTimeWasCalledNTimes)
	mockRecorder.Clear()
}

// Test_Controller_FailingGatherer tests that metadata file doesn't grow with failing gatherer functions
func Test_Controller_FailingGatherer(t *testing.T) {
	c, mockRecorder := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockFailingGatherer{},
	}, 3*time.Second)

	c.Gather()
	metadataFound := false
	assert.Len(t, mockRecorder.Records, 2)
	for i := range mockRecorder.Records {
		// find metadata record
		if mockRecorder.Records[i].Name != recorder.MetadataRecordName {
			continue
		}
		metadataFound = true
		b, err := mockRecorder.Records[i].Item.Marshal()
		assert.NoError(t, err)
		metaData := make(map[string]interface{})
		err = json.Unmarshal(b, &metaData)
		assert.NoError(t, err)
		assert.Len(t, metaData["status_reports"].([]interface{}), 2,
			fmt.Sprintf("Only one function for %s expected ", c.gatherers[0].GetName()))
	}
	assert.Truef(t, metadataFound, fmt.Sprintf("%s not found in records", recorder.MetadataRecordName))
	mockRecorder.Clear()
}

func getMocksForPeriodicTest(listGatherers []gatherers.Interface, interval time.Duration) (*Controller, *recorder.MockRecorder) {
	mockConfigurator := config.MockConfigurator{Conf: &config.Controller{
		Report:   true,
		Interval: interval,
		Gather:   []string{gather.AllGatherersConst},
	}}
	mockRecorder := recorder.MockRecorder{}

	return New(&mockConfigurator, nil, &mockRecorder, listGatherers, nil), &mockRecorder
}
