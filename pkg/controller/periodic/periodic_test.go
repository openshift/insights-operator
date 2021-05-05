package periodic

import (
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
	})

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
	})

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

// TODO: cover more things

func getMocksForPeriodicTest(gatherers []gatherers.Interface) (*Controller, *recorder.MockRecorder) {
	mockConfigurator := config.MockConfigurator{Conf: &config.Controller{
		Report:   true,
		Interval: time.Hour,
		Gather:   []string{gather.AllGatherersConst},
	}}
	mockRecorder := recorder.MockRecorder{}

	return New(&mockConfigurator, &mockRecorder, gatherers, nil), &mockRecorder
}
