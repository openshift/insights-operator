package periodic

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/api/config/v1alpha1"
	v1 "github.com/openshift/api/operator/v1"
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
			go c.Run(stopCh, tt.initialDelay)
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
			go c.periodicTrigger(stopCh)
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
	mockAPIConfigurator := config.NewMockAPIConfigurator(&v1alpha1.GatherConfig{})
	mockRecorder := recorder.MockRecorder{}
	mockAnonymizer, err := anonymization.NewAnonymizer("", []string{}, nil, &mockConfigurator, mockAPIConfigurator)
	if err != nil {
		return nil, nil, err
	}
	fakeInsightsOperatorCli := fakeOperatorCli.NewSimpleClientset().OperatorV1().InsightsOperators()
	mockController := New(&mockConfigurator, &mockRecorder, listGatherers, mockAnonymizer, fakeInsightsOperatorCli, mockAPIConfigurator)
	return mockController, &mockRecorder, nil
}

func Test_createGathererStatus(t *testing.T) { //nolint: funlen
	tests := []struct {
		name       string
		gfr        gather.GathererFunctionReport
		expectedGs v1.GathererStatus
	}{
		{
			name: "Data gathered OK",
			gfr: gather.GathererFunctionReport{
				FuncName:     "gatherer1/foo",
				Duration:     115000,
				RecordsCount: 5,
			},
			expectedGs: v1.GathererStatus{
				Name: "gatherer1/foo",
				LastGatherDuration: metav1.Duration{
					Duration: 115000000000,
				},
				Conditions: []metav1.Condition{
					{
						Type:    DataGatheredCondition,
						Status:  metav1.ConditionTrue,
						Reason:  GatheredOKReason,
						Message: "Created 5 records in the archive.",
					},
				},
			},
		},
		{
			name: "No Data",
			gfr: gather.GathererFunctionReport{
				FuncName:     "gatherer2/baz",
				Duration:     0,
				RecordsCount: 0,
			},
			expectedGs: v1.GathererStatus{
				Name: "gatherer2/baz",
				LastGatherDuration: metav1.Duration{
					Duration: 0,
				},
				Conditions: []metav1.Condition{
					{
						Type:   DataGatheredCondition,
						Status: metav1.ConditionFalse,
						Reason: NoDataGatheredReason,
					},
				},
			},
		},
		{
			name: "Gatherer Error",
			gfr: gather.GathererFunctionReport{
				FuncName:     "gatherer3/bar",
				Duration:     0,
				RecordsCount: 0,
				Errors:       []string{"unable to read the data"},
			},
			expectedGs: v1.GathererStatus{
				Name: "gatherer3/bar",
				LastGatherDuration: metav1.Duration{
					Duration: 0,
				},
				Conditions: []metav1.Condition{
					{
						Type:    DataGatheredCondition,
						Status:  metav1.ConditionFalse,
						Reason:  GatherErrorReason,
						Message: "unable to read the data",
					},
				},
			},
		},
		{
			name: "Data gathered with an error",
			gfr: gather.GathererFunctionReport{
				FuncName:     "gatherer4/quz",
				Duration:     9000,
				RecordsCount: 2,
				Errors:       []string{"didn't find xyz configmap"},
			},
			expectedGs: v1.GathererStatus{
				Name: "gatherer4/quz",
				LastGatherDuration: metav1.Duration{
					Duration: 9000000000,
				},
				Conditions: []metav1.Condition{
					{
						Type:    DataGatheredCondition,
						Status:  metav1.ConditionTrue,
						Reason:  GatheredWithErrorReason,
						Message: "Created 2 records in the archive. Error: didn't find xyz configmap",
					},
				},
			},
		},
		{
			name: "Gatherer panicked",
			gfr: gather.GathererFunctionReport{
				FuncName:     "gatherer5/quz",
				Duration:     0,
				RecordsCount: 0,
				Panic:        "quz gatherer panicked",
			},
			expectedGs: v1.GathererStatus{
				Name: "gatherer5/quz",
				LastGatherDuration: metav1.Duration{
					Duration: 0,
				},
				Conditions: []metav1.Condition{
					{
						Type:    DataGatheredCondition,
						Status:  metav1.ConditionFalse,
						Reason:  GatherPanicReason,
						Message: "quz gatherer panicked",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gathererStatus := createGathererStatus(&tt.gfr)
			assert.Equal(t, tt.expectedGs.Name, gathererStatus.Name)
			assert.Equal(t, tt.expectedGs.LastGatherDuration, gathererStatus.LastGatherDuration)

			// more asserts since we can use simple equal because of the last transition time of the condition
			assert.Len(t, gathererStatus.Conditions, 1)
			assert.Equal(t, tt.expectedGs.Conditions[0].Type, gathererStatus.Conditions[0].Type)
			assert.Equal(t, tt.expectedGs.Conditions[0].Reason, gathererStatus.Conditions[0].Reason)
			assert.Equal(t, tt.expectedGs.Conditions[0].Status, gathererStatus.Conditions[0].Status)
			assert.Equal(t, tt.expectedGs.Conditions[0].Message, gathererStatus.Conditions[0].Message)
		})
	}
}
