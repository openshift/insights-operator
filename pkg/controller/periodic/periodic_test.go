package periodic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	configv1alpha1 "github.com/openshift/api/config/v1alpha1"
	"github.com/openshift/api/insights/v1alpha1"
	operatorv1 "github.com/openshift/api/operator/v1"
	insightsFakeCli "github.com/openshift/client-go/insights/clientset/versioned/fake"
	fakeOperatorCli "github.com/openshift/client-go/operator/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/openshift/insights-operator/pkg/recorder"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
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
	// 5 gatherers + metadata (one is low priority, and we need to wait 999 hours to get it
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
	mockRecorder := recorder.MockRecorder{}
	mockAnonymizer, err := anonymization.NewAnonymizer("", []string{}, nil, &mockConfigurator, "")
	if err != nil {
		return nil, nil, err
	}
	fakeInsightsOperatorCli := fakeOperatorCli.NewSimpleClientset().OperatorV1().InsightsOperators()
	mockController := New(&mockConfigurator, &mockRecorder, listGatherers, mockAnonymizer, fakeInsightsOperatorCli, nil)
	return mockController, &mockRecorder, nil
}

func TestCreateNewDataGatherCR(t *testing.T) {
	cs := insightsFakeCli.NewSimpleClientset()
	mockController := NewWithTechPreview(nil, nil, nil, nil, nil, cs.InsightsV1alpha1(), nil, nil)
	tests := []struct {
		name              string
		disabledGatherers []string
		dataPolicy        v1alpha1.DataPolicy
		expected          *v1alpha1.DataGather
	}{
		{
			name:              "Empty DataGather resource creation",
			disabledGatherers: []string{},
			dataPolicy:        "",
			expected: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "periodic-gathering-",
				},
				Spec: v1alpha1.DataGatherSpec{
					DataPolicy: "",
				},
			},
		},
		{
			name:              "DataGather with NoPolicy DataPolicy",
			disabledGatherers: []string{},
			dataPolicy:        v1alpha1.NoPolicy,
			expected: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "periodic-gathering-",
				},
				Spec: v1alpha1.DataGatherSpec{
					DataPolicy: "ClearText",
				},
			},
		},
		{
			name: "DataGather with ObfuscateNetworking DataPolicy and some disabled gatherers",
			disabledGatherers: []string{
				"clusterconfig/foo",
				"clusterconfig/bar",
				"workloads",
			},
			dataPolicy: v1alpha1.ObfuscateNetworking,
			expected: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "periodic-gathering-",
				},
				Spec: v1alpha1.DataGatherSpec{
					DataPolicy: "ObfuscateNetworking",
					Gatherers: []v1alpha1.GathererConfig{
						{
							Name:  "clusterconfig/foo",
							State: v1alpha1.Disabled,
						},
						{
							Name:  "clusterconfig/bar",
							State: v1alpha1.Disabled,
						},
						{
							Name:  "workloads",
							State: v1alpha1.Disabled,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dg, err := mockController.createNewDataGatherCR(context.Background(), tt.disabledGatherers, tt.dataPolicy)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Spec, dg.Spec)
			err = cs.InsightsV1alpha1().DataGathers().Delete(context.Background(), dg.Name, metav1.DeleteOptions{})
			assert.NoError(t, err)
		})
	}
}

func TestUpdateNewDataGatherCRStatus(t *testing.T) {
	tests := []struct {
		name                     string
		testedDataGather         *v1alpha1.DataGather
		expectedDataRecordedCon  metav1.Condition
		expectedDataUploadedCon  metav1.Condition
		expectedDataProcessedCon metav1.Condition
	}{
		{
			name: "plain DataGather with no status",
			testedDataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-data-gather",
				},
			},
			expectedDataRecordedCon: metav1.Condition{
				Type:    status.DataRecorded,
				Status:  metav1.ConditionUnknown,
				Reason:  status.NoDataGatheringYetReason,
				Message: "",
			},
			expectedDataUploadedCon: metav1.Condition{
				Type:    status.DataRecorded,
				Status:  metav1.ConditionUnknown,
				Reason:  status.NoUploadYetReason,
				Message: "",
			},
			expectedDataProcessedCon: metav1.Condition{
				Type:    status.DataRecorded,
				Status:  metav1.ConditionUnknown,
				Reason:  status.NothingToProcessYetReason,
				Message: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset(tt.testedDataGather)
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, cs.InsightsV1alpha1(), nil, nil)
			err := mockController.updateNewDataGatherCRStatus(context.Background(), tt.testedDataGather.Name)
			assert.NoError(t, err)
			updatedDataGather, err := cs.InsightsV1alpha1().DataGathers().Get(context.Background(), tt.testedDataGather.Name, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Equal(t, v1alpha1.Pending, updatedDataGather.Status.State)

			dr := status.GetConditionByType(updatedDataGather, status.DataRecorded)
			assert.NotNil(t, dr)
			assert.Equal(t, tt.expectedDataRecordedCon.Status, dr.Status)
			assert.Equal(t, tt.expectedDataRecordedCon.Reason, dr.Reason)
			assert.Equal(t, tt.expectedDataRecordedCon.Message, dr.Message)

			du := status.GetConditionByType(updatedDataGather, status.DataUploaded)
			assert.NotNil(t, du)
			assert.Equal(t, tt.expectedDataUploadedCon.Status, du.Status)
			assert.Equal(t, tt.expectedDataUploadedCon.Reason, du.Reason)
			assert.Equal(t, tt.expectedDataUploadedCon.Message, du.Message)

			dp := status.GetConditionByType(updatedDataGather, status.DataProcessed)
			assert.NotNil(t, dp)
			assert.Equal(t, tt.expectedDataProcessedCon.Status, dp.Status)
			assert.Equal(t, tt.expectedDataProcessedCon.Reason, dp.Reason)
			assert.Equal(t, tt.expectedDataProcessedCon.Message, dp.Message)
		})
	}
}

func TestCopyDataGatherStatusToOperatorStatus(t *testing.T) {
	tests := []struct {
		name                   string
		testedDataGather       *v1alpha1.DataGather
		testedInsightsOperator operatorv1.InsightsOperator
		expected               *operatorv1.InsightsOperator
	}{
		{
			name: "Basic copy status test",
			testedDataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Status: v1alpha1.DataGatherStatus{
					State:      v1alpha1.Failed,
					StartTime:  metav1.Date(2020, 5, 13, 2, 30, 0, 0, time.UTC),
					FinishTime: metav1.Date(2020, 5, 13, 2, 56, 54, 0, time.UTC),
					Gatherers: []v1alpha1.GathererStatus{
						{
							Name: "clusterconfig/foo1",
							Conditions: []metav1.Condition{
								{
									Type:               status.DataGatheredCondition,
									Status:             metav1.ConditionTrue,
									Reason:             status.GatheredOKReason,
									LastTransitionTime: metav1.Date(2020, 5, 13, 2, 35, 5, 0, time.UTC),
								},
							},
						},
						{
							Name: "clusterconfig/bar",
							Conditions: []metav1.Condition{
								{
									Type:               status.DataGatheredCondition,
									Status:             metav1.ConditionTrue,
									Reason:             status.GatherErrorReason,
									Message:            "Gatherer failed",
									LastTransitionTime: metav1.Date(2020, 5, 13, 2, 36, 5, 0, time.UTC),
								},
							},
						},
						{
							Name: "workloads",
							Conditions: []metav1.Condition{
								{
									Type:               status.DataGatheredCondition,
									Status:             metav1.ConditionTrue,
									Reason:             status.GatheredOKReason,
									LastTransitionTime: metav1.Date(2020, 5, 13, 2, 38, 5, 0, time.UTC),
								},
							},
						},
					},
				},
			},
			testedInsightsOperator: operatorv1.InsightsOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: operatorv1.InsightsOperatorStatus{
					GatherStatus: operatorv1.GatherStatus{
						LastGatherTime:     metav1.Date(2020, 5, 12, 2, 0, 0, 0, time.UTC),
						LastGatherDuration: metav1.Duration{Duration: 5 * time.Minute},
						Gatherers: []operatorv1.GathererStatus{
							{
								Name: "clusterconfig/foo1",
								Conditions: []metav1.Condition{
									{
										Type:               status.DataGatheredCondition,
										Status:             metav1.ConditionTrue,
										Reason:             status.GatheredOKReason,
										LastTransitionTime: metav1.Date(2020, 5, 12, 1, 0, 0, 0, time.UTC),
									},
								},
							},
						},
					},
				},
			},
			expected: &operatorv1.InsightsOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: operatorv1.InsightsOperatorStatus{
					GatherStatus: operatorv1.GatherStatus{
						LastGatherTime: metav1.Date(2020, 5, 13, 2, 56, 54, 0, time.UTC),
						LastGatherDuration: metav1.Duration{
							Duration: 1614 * time.Second,
						},
						Gatherers: []operatorv1.GathererStatus{
							{
								Name: "clusterconfig/foo1",
								Conditions: []metav1.Condition{
									{
										Type:               status.DataGatheredCondition,
										Status:             metav1.ConditionTrue,
										Reason:             status.GatheredOKReason,
										LastTransitionTime: metav1.Date(2020, 5, 13, 2, 35, 5, 0, time.UTC),
									},
								},
							},
							{
								Name: "clusterconfig/bar",
								Conditions: []metav1.Condition{
									{
										Type:               status.DataGatheredCondition,
										Status:             metav1.ConditionTrue,
										Reason:             status.GatherErrorReason,
										Message:            "Gatherer failed",
										LastTransitionTime: metav1.Date(2020, 5, 13, 2, 36, 5, 0, time.UTC),
									},
								},
							},
							{
								Name: "workloads",
								Conditions: []metav1.Condition{
									{
										Type:               status.DataGatheredCondition,
										Status:             metav1.ConditionTrue,
										Reason:             status.GatheredOKReason,
										LastTransitionTime: metav1.Date(2020, 5, 13, 2, 38, 5, 0, time.UTC),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "InsightsReport attribute is updated when copying",
			testedDataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Status: v1alpha1.DataGatherStatus{
					State:      v1alpha1.Failed,
					StartTime:  metav1.Date(2020, 5, 13, 2, 30, 0, 0, time.UTC),
					FinishTime: metav1.Date(2020, 5, 13, 2, 56, 54, 0, time.UTC),
					Gatherers: []v1alpha1.GathererStatus{
						{
							Name: "clusterconfig/foo1",
							Conditions: []metav1.Condition{
								{
									Type:               status.DataGatheredCondition,
									Status:             metav1.ConditionTrue,
									Reason:             status.GatheredOKReason,
									LastTransitionTime: metav1.Date(2020, 5, 13, 2, 35, 5, 0, time.UTC),
								},
							},
						},
					},
					InsightsReport: v1alpha1.InsightsReport{
						DownloadedAt: metav1.Date(2020, 5, 13, 2, 40, 0, 0, time.UTC),
						HealthChecks: []v1alpha1.HealthCheck{
							{
								Description: "healtheck ABC",
								TotalRisk:   1,
								State:       v1alpha1.HealthCheckEnabled,
								AdvisorURI:  "test-uri",
							},
							{
								Description: "healtheck XYZ",
								TotalRisk:   2,
								State:       v1alpha1.HealthCheckEnabled,
								AdvisorURI:  "test-uri-1",
							},
						},
					},
				},
			},
			testedInsightsOperator: operatorv1.InsightsOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: operatorv1.InsightsOperatorStatus{
					InsightsReport: operatorv1.InsightsReport{},
					GatherStatus: operatorv1.GatherStatus{
						LastGatherTime:     metav1.Date(2020, 5, 12, 2, 0, 0, 0, time.UTC),
						LastGatherDuration: metav1.Duration{Duration: 5 * time.Minute},
						Gatherers: []operatorv1.GathererStatus{
							{
								Name: "clusterconfig/foo1",
								Conditions: []metav1.Condition{
									{
										Type:               status.DataGatheredCondition,
										Status:             metav1.ConditionTrue,
										Reason:             status.GatheredOKReason,
										LastTransitionTime: metav1.Date(2020, 5, 12, 1, 0, 0, 0, time.UTC),
									},
								},
							},
						},
					},
				},
			},
			expected: &operatorv1.InsightsOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: operatorv1.InsightsOperatorStatus{
					InsightsReport: operatorv1.InsightsReport{
						DownloadedAt: metav1.Date(2020, 5, 13, 2, 40, 0, 0, time.UTC),
						HealthChecks: []operatorv1.HealthCheck{
							{
								Description: "healtheck ABC",
								TotalRisk:   1,
								State:       operatorv1.HealthCheckEnabled,
								AdvisorURI:  "test-uri",
							},
							{
								Description: "healtheck XYZ",
								TotalRisk:   2,
								State:       operatorv1.HealthCheckEnabled,
								AdvisorURI:  "test-uri-1",
							},
						},
					},
					GatherStatus: operatorv1.GatherStatus{
						LastGatherTime: metav1.Date(2020, 5, 13, 2, 56, 54, 0, time.UTC),
						LastGatherDuration: metav1.Duration{
							Duration: 1614 * time.Second,
						},
						Gatherers: []operatorv1.GathererStatus{
							{
								Name: "clusterconfig/foo1",
								Conditions: []metav1.Condition{
									{
										Type:               status.DataGatheredCondition,
										Status:             metav1.ConditionTrue,
										Reason:             status.GatheredOKReason,
										LastTransitionTime: metav1.Date(2020, 5, 13, 2, 35, 5, 0, time.UTC),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataGatherFakeCS := insightsFakeCli.NewSimpleClientset(tt.testedDataGather)
			operatorFakeCS := fakeOperatorCli.NewSimpleClientset(&tt.testedInsightsOperator)
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil,
				dataGatherFakeCS.InsightsV1alpha1(), operatorFakeCS.OperatorV1().InsightsOperators(), nil)
			updatedOperator, err := mockController.copyDataGatherStatusToOperatorStatus(context.Background(), tt.testedDataGather)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, updatedOperator)
		})
	}
}

func TestCreateDataGatherAttributeValues(t *testing.T) {
	tests := []struct {
		name                      string
		gatherConfig              configv1alpha1.GatherConfig
		gatheres                  []gatherers.Interface
		expectedPolicy            v1alpha1.DataPolicy
		expectedDisabledGatherers []string
	}{
		{
			name: "Two disabled gatherers and ObfuscateNetworking Policy",
			gatherConfig: configv1alpha1.GatherConfig{
				DataPolicy: configv1alpha1.ObfuscateNetworking,
				DisabledGatherers: []string{
					"mock_gatherer",
					"foo_gatherer",
				},
			},
			gatheres: []gatherers.Interface{
				&gather.MockGatherer{},
				&gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true},
			},
			expectedPolicy:            v1alpha1.ObfuscateNetworking,
			expectedDisabledGatherers: []string{"mock_gatherer", "foo_gatherer"},
		},
		{
			name: "Custom period gatherer is excluded because it should not be processed",
			gatherConfig: configv1alpha1.GatherConfig{
				DataPolicy: configv1alpha1.NoPolicy,
				DisabledGatherers: []string{
					"clusterconfig/bar",
				},
			},
			gatheres: []gatherers.Interface{
				&gather.MockGatherer{},
				&gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: false},
			},
			expectedPolicy:            v1alpha1.NoPolicy,
			expectedDisabledGatherers: []string{"clusterconfig/bar", "mock_custom_period_gatherer_no_period"},
		},
		{
			name: "Empty data policy is created as NoPolicy/ClearText",
			gatherConfig: configv1alpha1.GatherConfig{
				DataPolicy:        "",
				DisabledGatherers: []string{},
			},
			gatheres: []gatherers.Interface{
				&gather.MockGatherer{},
				&gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true},
			},
			expectedPolicy:            v1alpha1.NoPolicy,
			expectedDisabledGatherers: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPIConfig := config.NewMockAPIConfigurator(&tt.gatherConfig)
			mockController := NewWithTechPreview(nil, nil, mockAPIConfig, tt.gatheres, nil, nil, nil, nil)
			disabledGatherers, dp := mockController.createDataGatherAttributeValues()
			assert.Equal(t, tt.expectedPolicy, dp)
			assert.EqualValues(t, disabledGatherers, tt.expectedDisabledGatherers)
		})
	}
}

func TestGetInsightsImage(t *testing.T) {
	tests := []struct {
		name              string
		testDeployment    appsv1.Deployment
		expectedImageName string
		expectedError     error
	}{
		{
			name: "Successful image get",
			testDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "insights-operator",
					Namespace: insightsNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "test-image",
									Image: "testing-image:123",
								},
							},
						},
					},
				},
			},
			expectedImageName: "testing-image:123",
			expectedError:     nil,
		},
		{
			name: "Empty deployment spec",
			testDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "insights-operator",
					Namespace: insightsNamespace,
				},
				Spec: appsv1.DeploymentSpec{},
			},
			expectedImageName: "",
			expectedError:     fmt.Errorf("no container defined in the deployment"),
		},
		{
			name: "Multiple containers - first container image is returned",
			testDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "insights-operator",
					Namespace: insightsNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "test-image-1",
									Image: "testing-image-1:123",
								},
								{
									Name:  "test-image-2",
									Image: "testing-image-1:123",
								},
							},
						},
					},
				},
			},
			expectedImageName: "testing-image-1:123",
			expectedError:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := kubefake.NewSimpleClientset(&tt.testDeployment)
			mockController := NewWithTechPreview(nil, nil, nil, nil, cs, nil, nil, nil)
			imgName, err := mockController.getInsightsImage(context.Background())
			assert.Equal(t, tt.expectedError, err)
			assert.Equal(t, tt.expectedImageName, imgName)
		})
	}
}

func TestPeriodicPrune(t *testing.T) {
	tests := []struct {
		name                string
		jobs                []runtime.Object
		dataGathers         []runtime.Object
		expectedJobs        []string
		expectedDataGathers []string
	}{
		{
			name: "Basic pruning test",
			jobs: []runtime.Object{
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "to-be-removed-job-1",
						Namespace: insightsNamespace,
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-25 * time.Hour),
						},
					},
				},
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "to-be-removed-job-2",
						Namespace: insightsNamespace,
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-1441 * time.Minute),
						},
					},
				},
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "to-keep-job-1",
						Namespace: insightsNamespace,
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-23 * time.Hour),
						},
					},
				},
				&batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "to-keep-job-2",
						Namespace: insightsNamespace,
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-2 * time.Hour),
						},
					},
				},
			},
			dataGathers: []runtime.Object{
				&v1alpha1.DataGather{
					ObjectMeta: metav1.ObjectMeta{
						Name: "to-be-removed-dg-1",
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-25 * time.Hour),
						},
					},
				},
				&v1alpha1.DataGather{
					ObjectMeta: metav1.ObjectMeta{
						Name: "to-be-removed-dg-2",
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-1441 * time.Minute),
						},
					},
				},
				&v1alpha1.DataGather{
					ObjectMeta: metav1.ObjectMeta{
						Name: "to-keep-dg-1",
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-1339 * time.Minute),
						},
					},
				},
			},
			expectedJobs:        []string{"to-keep-job-1", "to-keep-job-2"},
			expectedDataGathers: []string{"to-keep-dg-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeCs := kubefake.NewSimpleClientset(tt.jobs...)
			insightsCs := insightsFakeCli.NewSimpleClientset(tt.dataGathers...)
			mockController := NewWithTechPreview(nil, nil, nil, nil, kubeCs, insightsCs.InsightsV1alpha1(), nil, nil)
			mockController.pruneInterval = 90 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			mockController.PeriodicPrune(ctx)

			jobList, err := kubeCs.BatchV1().Jobs(insightsNamespace).List(context.Background(), metav1.ListOptions{})
			assert.NoError(t, err)
			assert.Len(t, jobList.Items, 2)
			for _, j := range jobList.Items {
				assert.Contains(t, tt.expectedJobs, j.Name)
			}
			dataGathersList, err := insightsCs.InsightsV1alpha1().DataGathers().List(context.Background(), metav1.ListOptions{})
			assert.NoError(t, err)
			assert.Len(t, dataGathersList.Items, 1)
			for _, dg := range dataGathersList.Items {
				assert.Contains(t, tt.expectedDataGathers, dg.Name)
			}
		})
	}
}

func TestWasDataUploaded(t *testing.T) {
	tests := []struct {
		name             string
		testedDataGather *v1alpha1.DataGather
		expectedSummary  controllerstatus.Summary
	}{
		{
			name: "Data gather was successful",
			testedDataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
					},
				},
			},
			expectedSummary: controllerstatus.Summary{
				Operation: controllerstatus.Uploading,
				Healthy:   true,
				Count:     1,
			},
		},
		{
			name: "Data gather not successful - upload failed",
			testedDataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:    status.DataUploaded,
							Status:  metav1.ConditionFalse,
							Reason:  "NotAuthorized",
							Message: "test error message",
						},
					},
				},
			},
			expectedSummary: controllerstatus.Summary{
				Operation: controllerstatus.Uploading,
				Healthy:   false,
				Count:     5,
				Reason:    "NotAuthorized",
				Message:   "test error message",
			},
		},
		{
			name: "Data gather missing DataUploaded condition",
			testedDataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-dg",
				},
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:    status.DataRecorded,
							Status:  metav1.ConditionFalse,
							Reason:  "ERROR",
							Message: "test error message",
						},
					},
				},
			},
			expectedSummary: controllerstatus.Summary{
				Operation: controllerstatus.Uploading,
				Healthy:   false,
				Count:     5,
				Reason:    dataUplodedConditionNotAvailable,
				Message: fmt.Sprintf("did not find any %q condition in the test-dg dataGather resource",
					status.DataUploaded),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, nil, nil, nil)
			successful := mockController.wasDataUploaded(tt.testedDataGather)
			assert.Equal(t, tt.expectedSummary.Healthy, successful)
			summary, _ := mockController.Sources()[0].CurrentStatus()
			assert.Equal(t, tt.expectedSummary, summary)
		})
	}
}

func TestWasDataProcessed(t *testing.T) {
	tests := []struct {
		name              string
		dataGather        *v1alpha1.DataGather
		expectedProcessed bool
	}{
		{
			name: "Empty conditions - not processed",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expectedProcessed: false,
		},
		{
			name: "DataProcessed status unknown - not processed",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						status.DataProcessedCondition(metav1.ConditionUnknown, status.NothingToProcessYetReason, ""),
					},
				},
			},
			expectedProcessed: false,
		},
		{
			name: "DataProcessed status true - processed",
			dataGather: &v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						status.DataProcessedCondition(metav1.ConditionTrue, "Processed", ""),
					},
				},
			},
			expectedProcessed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processed := wasDataProcessed(tt.dataGather)
			assert.Equal(t, tt.expectedProcessed, processed)
		})
	}
}

func TestUpdateInsightsReportInDataGather(t *testing.T) {
	tests := []struct {
		name                   string
		dataGatherToUpdate     *v1alpha1.DataGather
		analysisReport         *types.InsightsAnalysisReport
		expectedInsightsReport *v1alpha1.InsightsReport
	}{
		{
			name: "DataGather is updated with active recommendations",
			dataGatherToUpdate: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-data-gather",
				},
			},
			analysisReport: &types.InsightsAnalysisReport{
				DownloadedAt: metav1.Date(2022, 11, 24, 5, 40, 0, 0, time.UTC),
				ClusterID:    "test-cluster-id",
				Recommendations: []types.Recommendation{
					{
						ErrorKey:    "test-error-key-1",
						Description: "lorem-ipsum",
						TotalRisk:   1,
						RuleFQDN:    "test.fqdn.key1",
					},
					{
						ErrorKey:    "test-error-key-2",
						Description: "lorem-ipsum bla bla test",
						TotalRisk:   4,
						RuleFQDN:    "test.fqdn.key2",
					},
				},
				RequestID: "test-request-id",
			},
			expectedInsightsReport: &v1alpha1.InsightsReport{
				DownloadedAt: metav1.Date(2022, 11, 24, 5, 40, 0, 0, time.UTC),
				URI:          "https://test.report.endpoint.tech.preview.uri/cluster/test-cluster-id/requestID/test-request-id",
				HealthChecks: []v1alpha1.HealthCheck{
					{
						Description: "lorem-ipsum",
						TotalRisk:   1,
						AdvisorURI:  "https://console.redhat.com/openshift/insights/advisor/clusters/test-cluster-id?first=test.fqdn.key1|test-error-key-1",
						State:       v1alpha1.HealthCheckEnabled,
					},
					{
						Description: "lorem-ipsum bla bla test",
						TotalRisk:   4,
						AdvisorURI:  "https://console.redhat.com/openshift/insights/advisor/clusters/test-cluster-id?first=test.fqdn.key2|test-error-key-2",
						State:       v1alpha1.HealthCheckEnabled,
					},
				},
			},
		},
		{
			name: "No active recommendations",
			dataGatherToUpdate: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-data-gather",
				},
			},
			analysisReport: &types.InsightsAnalysisReport{
				DownloadedAt:    metav1.Date(2022, 11, 24, 5, 40, 0, 0, time.UTC),
				ClusterID:       "test-cluster-id",
				Recommendations: []types.Recommendation{},
				RequestID:       "test-request-id",
			},
			expectedInsightsReport: &v1alpha1.InsightsReport{
				DownloadedAt: metav1.Date(2022, 11, 24, 5, 40, 0, 0, time.UTC),
				URI:          "https://test.report.endpoint.tech.preview.uri/cluster/test-cluster-id/requestID/test-request-id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insightsCs := insightsFakeCli.NewSimpleClientset(tt.dataGatherToUpdate)
			mockSecretConf := &config.MockSecretConfigurator{
				Conf: &config.Controller{
					ReportEndpointTechPreview: "https://test.report.endpoint.tech.preview.uri/cluster/%s/requestID/%s",
				},
			}
			mockController := NewWithTechPreview(nil, mockSecretConf, nil, nil, nil, insightsCs.InsightsV1alpha1(), nil, nil)
			err := mockController.updateInsightsReportInDataGather(context.Background(), tt.analysisReport, tt.dataGatherToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedInsightsReport, &tt.dataGatherToUpdate.Status.InsightsReport)
		})
	}
}

func TestDataGatherState(t *testing.T) {
	tests := []struct {
		name           string
		dataGatherName string
		dataGather     *v1alpha1.DataGather
		expectedState  v1alpha1.DataGatherState
		err            error
	}{
		{
			name:           "Existing DataGather state is read correctly",
			dataGatherName: "test-dg",
			dataGather: &v1alpha1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-dg",
				},
				Status: v1alpha1.DataGatherStatus{
					State: v1alpha1.Pending,
				},
			},
			expectedState: v1alpha1.Pending,
			err:           nil,
		},
		{
			name:           "Non-existing DataGather state returns an error",
			dataGatherName: "non-existing",
			dataGather:     nil,
			expectedState:  v1alpha1.Pending,
			err: errors.NewNotFound(schema.GroupResource{
				Group:    "insights.openshift.io",
				Resource: "datagathers",
			}, "non-existing"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var insightsCs *insightsFakeCli.Clientset
			if tt.err != nil {
				insightsCs = insightsFakeCli.NewSimpleClientset()
			} else {
				insightsCs = insightsFakeCli.NewSimpleClientset(tt.dataGather)
			}
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, insightsCs.InsightsV1alpha1(), nil, nil)
			state, err := mockController.dataGatherState(context.Background(), tt.dataGatherName)
			if tt.err != nil {
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.dataGather.Status.State, state)
			}
		})
	}
}
