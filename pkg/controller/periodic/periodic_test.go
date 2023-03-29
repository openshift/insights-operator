package periodic

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	fakeOperatorCli "github.com/openshift/client-go/operator/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/recorder"
)

func Test_Controller_CustomPeriodGatherer(t *testing.T) {
	c, mockRecorder, err := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{},
		&gather.MockCustomPeriodGatherer{Period: 999 * time.Hour},
	}, 1*time.Hour)
	assert.NoError(t, err)
	c.Gather()
	// 6 gatherers + metadata
	assert.Len(t, mockRecorder.Records, 7)
	mockRecorder.Reset()

	c.Gather()
	// 5 gatherers + metadata (one is low priority and we need to wait 999 hours to get it
	assert.Len(t, mockRecorder.Records, 6)
	mockRecorder.Reset()
}

func Test_Controller_Run(t *testing.T) {
	tests := []struct {
		name                 string
		initialDelay         time.Duration
		waitTime             time.Duration
		expectedNumOfRecords int
	}{
		{
			name:                 "controller run with no initial delay",
			initialDelay:         0,
			waitTime:             100 * time.Millisecond,
			expectedNumOfRecords: 6,
		},
		{
			name:                 "controller run with short initial delay",
			initialDelay:         2 * time.Second,
			waitTime:             4 * time.Second,
			expectedNumOfRecords: 6,
		},
		{
			name:                 "controller run stop before delay ends",
			initialDelay:         2 * time.Hour,
			waitTime:             1 * time.Second,
			expectedNumOfRecords: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockRecorder, err := getMocksForPeriodicTest([]gatherers.Interface{
				&gather.MockGatherer{},
			}, 1*time.Hour)
			assert.NoError(t, err)
			stopCh := make(chan struct{})
			go c.Run(stopCh, tt.initialDelay, false)
			if _, ok := <-time.After(tt.waitTime); ok {
				stopCh <- struct{}{}
			}
			assert.Len(t, mockRecorder.Records, tt.expectedNumOfRecords)
		})
	}
}

func Test_Controller_periodicTrigger(t *testing.T) {
	tests := []struct {
		name                 string
		interval             time.Duration
		waitTime             time.Duration
		expectedNumOfRecords int
	}{
		{
			name:                 "periodicTrigger finished gathering",
			interval:             1 * time.Second,
			waitTime:             3 * time.Second,
			expectedNumOfRecords: 12,
		},
		{
			name:                 "periodicTrigger stopped with no data gathered",
			interval:             2 * time.Hour,
			waitTime:             100 * time.Millisecond,
			expectedNumOfRecords: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockRecorder, err := getMocksForPeriodicTest([]gatherers.Interface{
				&gather.MockGatherer{},
			}, tt.interval)
			assert.NoError(t, err)
			stopCh := make(chan struct{})
			go c.periodicTrigger(stopCh, false)
			if _, ok := <-time.After(tt.waitTime); ok {
				stopCh <- struct{}{}
			}
			assert.Len(t, mockRecorder.Records, tt.expectedNumOfRecords)
		})
	}
}

func Test_Controller_Sources(t *testing.T) {
	mockGatherer := gather.MockGatherer{}
	mockCustomPeriodGatherer := gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true}
	// 1 Gatherer ==> 1 source
	c, _, _ := getMocksForPeriodicTest([]gatherers.Interface{
		&mockGatherer,
	}, 1*time.Hour)
	assert.Len(t, c.Sources(), 1)

	// 2 Gatherer ==> 2 source
	c, _, _ = getMocksForPeriodicTest([]gatherers.Interface{
		&mockGatherer,
		&mockCustomPeriodGatherer,
	}, 1*time.Hour)
	assert.Len(t, c.Sources(), 2)
}

func Test_Controller_CustomPeriodGathererNoPeriod(t *testing.T) {
	mockGatherer := gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true}
	c, mockRecorder, err := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockGatherer{},
		&mockGatherer,
	}, 1*time.Hour)
	assert.NoError(t, err)
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
	c, mockRecorder, err := getMocksForPeriodicTest([]gatherers.Interface{
		&gather.MockFailingGatherer{},
	}, 3*time.Second)
	assert.NoError(t, err)
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
	mockRecorder.Reset()
}

func getMocksForPeriodicTest(listGatherers []gatherers.Interface, interval time.Duration) (*Controller, *recorder.MockRecorder, error) {
	mockConfigurator := config.MockSecretConfigurator{Conf: &config.Controller{
		Report:   true,
		Interval: interval,
	}}
	mockRecorder := recorder.MockRecorder{}
	mockAnonymizer, err := anonymization.NewAnonymizer("", []string{}, nil, &mockConfigurator, "")
	if err != nil {
		return nil, nil, err
	}
	fakeInsightsOperatorCli := fakeOperatorCli.NewSimpleClientset().OperatorV1().InsightsOperators()
	mockController := New(&mockConfigurator, &mockRecorder, listGatherers, mockAnonymizer, fakeInsightsOperatorCli, nil)
	return mockController, &mockRecorder, nil
}
