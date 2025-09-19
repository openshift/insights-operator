package periodic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	configv1 "github.com/openshift/api/config/v1"
	configv1alpha2 "github.com/openshift/api/config/v1alpha2"
	"github.com/openshift/api/insights/v1alpha2"
	operatorv1 "github.com/openshift/api/operator/v1"
	configFakeCli "github.com/openshift/client-go/config/clientset/versioned/fake"
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
			interval:             2 * time.Second,
			waitTime:             3 * time.Second,
			expectedNumOfRecords: 6,
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
	mockConfigMapConfigurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			Enabled:  true,
			Interval: interval,
		},
	})
	mockRecorder := recorder.MockRecorder{}
	anonBuilder := &anonymization.NetworkAnonymizerBuilder{}
	mockNetworkAnonymizer, err := anonBuilder.WithConfigurator(mockConfigMapConfigurator).Build()
	if err != nil {
		return nil, nil, err
	}

	mockAnonymizer, err := anonymization.NewAnonymizer(mockNetworkAnonymizer)
	if err != nil {
		return nil, nil, err
	}

	fakeInsightsOperatorCli := fakeOperatorCli.NewSimpleClientset().OperatorV1().InsightsOperators()
	mockController := New(mockConfigMapConfigurator, &mockRecorder, listGatherers, mockAnonymizer, fakeInsightsOperatorCli, nil)
	return mockController, &mockRecorder, nil
}

func TestCreateNewDataGatherCR(t *testing.T) {
	cs := insightsFakeCli.NewSimpleClientset()
	tests := []struct {
		name           string
		dataPolicy     []configv1alpha2.DataPolicyOption
		configGatherer configv1alpha2.Gatherers
		expected       *v1alpha2.DataGather
	}{
		{
			name:       "DataGather with enabled gathering",
			dataPolicy: []configv1alpha2.DataPolicyOption{},
			configGatherer: configv1alpha2.Gatherers{
				Mode: configv1alpha2.GatheringModeAll,
			},
			expected: &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "periodic-gathering-",
				},
				Spec: v1alpha2.DataGatherSpec{
					Gatherers: &v1alpha2.Gatherers{
						Mode: v1alpha2.GatheringModeAll,
					},
				},
			},
		},
		{
			name:       "DataGather with ObfuscateNetworking DataPolicy and custom gathering configs",
			dataPolicy: []configv1alpha2.DataPolicyOption{configv1alpha2.DataPolicyOptionObfuscateNetworking},
			configGatherer: configv1alpha2.Gatherers{
				Mode: configv1alpha2.GatheringModeCustom,
				Custom: &configv1alpha2.Custom{
					Configs: []configv1alpha2.GathererConfig{
						{
							Name:  "clusterconfig/foo",
							State: configv1alpha2.GathererStateDisabled,
						},
						{
							Name:  "clusterconfig/bar",
							State: configv1alpha2.GathererStateDisabled,
						},
						{
							Name:  "workloads",
							State: configv1alpha2.GathererStateDisabled,
						},
					},
				},
			},
			expected: &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "periodic-gathering-",
				},
				Spec: v1alpha2.DataGatherSpec{
					DataPolicy: []v1alpha2.DataPolicyOption{
						v1alpha2.DataPolicyOptionObfuscateNetworking,
					},
					Gatherers: &v1alpha2.Gatherers{
						Mode: v1alpha2.GatheringModeCustom,
						Custom: &v1alpha2.Custom{
							Configs: []v1alpha2.GathererConfig{
								{
									Name:  "clusterconfig/foo",
									State: v1alpha2.GathererStateDisabled,
								},
								{
									Name:  "clusterconfig/bar",
									State: v1alpha2.GathererStateDisabled,
								},
								{
									Name:  "workloads",
									State: v1alpha2.GathererStateDisabled,
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
			apiConfig := NewInsightsDataGatherObserverMock(
				tt.dataPolicy,
				tt.configGatherer,
			)
			mockController := NewWithTechPreview(nil, nil, apiConfig, nil, nil, cs.InsightsV1alpha2(), nil, nil, nil)

			dg, err := mockController.createNewDataGatherCR(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Spec, dg.Spec)
			assert.Equal(t, tt.expected.Spec.Gatherers, dg.Spec.Gatherers)
			assert.Equal(t, tt.expected.Spec.DataPolicy, dg.Spec.DataPolicy)
			assert.Equal(t, tt.expected.Spec.Storage, dg.Spec.Storage)

			err = cs.InsightsV1alpha2().DataGathers().Delete(context.Background(), dg.Name, metav1.DeleteOptions{})
			assert.NoError(t, err)
		})
	}
}

func TestUpdateNewDataGatherCRStatus(t *testing.T) {
	tests := []struct {
		name                         string
		testedDataGather             *v1alpha2.DataGather
		testJob                      *batchv1.Job
		expectedDataRecordedCon      metav1.Condition
		expectedDataUploadedCon      metav1.Condition
		expectedDataProcessedCon     metav1.Condition
		expectedProgressingCondition metav1.Condition
		expectedObjectReference      v1alpha2.ObjectReference
	}{
		{
			name: "plain DataGather with no status",
			testedDataGather: &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-data-gather",
				},
			},
			testJob: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-data-gather-job",
					Namespace: "test-namespace",
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
			expectedProgressingCondition: metav1.Condition{
				Type:    status.Progressing,
				Status:  metav1.ConditionFalse,
				Reason:  status.DataGatheringPendingReason,
				Message: status.DataGatheringPendingMessage,
			},
			expectedObjectReference: v1alpha2.ObjectReference{
				Group:     "batch",
				Resource:  "job",
				Name:      "test-data-gather-job",
				Namespace: "test-namespace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := insightsFakeCli.NewSimpleClientset(tt.testedDataGather)
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, cs.InsightsV1alpha2(), nil, nil, nil)
			err := mockController.updateNewDataGatherCRStatus(context.Background(), tt.testedDataGather, tt.testJob)
			assert.NoError(t, err)
			updatedDataGather, err := cs.InsightsV1alpha2().DataGathers().Get(context.Background(), tt.testedDataGather.Name, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Len(t, updatedDataGather.Status.RelatedObjects, 1)
			assert.Equal(t, tt.expectedObjectReference, updatedDataGather.Status.RelatedObjects[0])

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

			progressingCondition := status.GetConditionByType(updatedDataGather, status.Progressing)
			assert.NotNil(t, progressingCondition)
			assert.Equal(t, tt.expectedProgressingCondition.Status, progressingCondition.Status)
			assert.Equal(t, tt.expectedProgressingCondition.Reason, progressingCondition.Reason)
			assert.Equal(t, tt.expectedProgressingCondition.Message, progressingCondition.Message)
		})
	}
}

func TestCopyDataGatherStatusToOperatorStatus(t *testing.T) {
	tests := []struct {
		name                   string
		testedDataGather       *v1alpha2.DataGather
		testedInsightsOperator operatorv1.InsightsOperator
		expected               *operatorv1.InsightsOperator
	}{
		{
			name: "Basic copy status test",
			testedDataGather: &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Status: v1alpha2.DataGatherStatus{
					StartTime:  ptr.To(metav1.Date(2020, 5, 13, 2, 30, 0, 0, time.UTC)),
					FinishTime: ptr.To(metav1.Date(2020, 5, 13, 2, 56, 54, 0, time.UTC)),
					Gatherers: []v1alpha2.GathererStatus{
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
			testedDataGather: &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Status: v1alpha2.DataGatherStatus{
					StartTime:  ptr.To(metav1.Date(2020, 5, 13, 2, 30, 0, 0, time.UTC)),
					FinishTime: ptr.To(metav1.Date(2020, 5, 13, 2, 56, 54, 0, time.UTC)),
					Gatherers: []v1alpha2.GathererStatus{
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
					InsightsReport: v1alpha2.InsightsReport{
						DownloadedTime: ptr.To(metav1.Date(2020, 5, 13, 2, 40, 0, 0, time.UTC)),
						HealthChecks: []v1alpha2.HealthCheck{
							{
								Description: "healtheck ABC",
								TotalRisk:   v1alpha2.TotalRiskLow,
								AdvisorURI:  "test-uri",
							},
							{
								Description: "healtheck XYZ",
								TotalRisk:   v1alpha2.TotalRiskModerate,
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
				dataGatherFakeCS.InsightsV1alpha2(), operatorFakeCS.OperatorV1().InsightsOperators(), nil, nil)
			updatedOperator, err := mockController.copyDataGatherStatusToOperatorStatus(context.Background(), tt.testedDataGather)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, updatedOperator)
		})
	}
}

func TestCreateDataGatherAttributeValues(t *testing.T) {
	tests := []struct {
		name                      string
		gatherConfig              configv1alpha2.GatherConfig
		gatheres                  []gatherers.Interface
		expectedPolicy            []v1alpha2.DataPolicyOption
		expectedDisabledGatherers v1alpha2.Gatherers
	}{
		{
			name: "Two disabled gatherers and ObfuscateNetworking Policy",
			gatherConfig: configv1alpha2.GatherConfig{
				DataPolicy: []configv1alpha2.DataPolicyOption{
					configv1alpha2.DataPolicyOptionObfuscateNetworking,
				},
				Gatherers: configv1alpha2.Gatherers{
					Mode: configv1alpha2.GatheringModeCustom,
					Custom: &configv1alpha2.Custom{
						Configs: []configv1alpha2.GathererConfig{
							{
								Name:  "mock_gatherer",
								State: configv1alpha2.GathererStateDisabled,
							},
							{
								Name:  "foo_gatherer",
								State: configv1alpha2.GathererStateDisabled,
							},
						},
					},
				},
				Storage: nil,
			},
			gatheres: []gatherers.Interface{
				&gather.MockGatherer{},
				&gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: true},
			},
			expectedPolicy: []v1alpha2.DataPolicyOption{
				v1alpha2.DataPolicyOptionObfuscateNetworking,
			},
			expectedDisabledGatherers: v1alpha2.Gatherers{
				Mode: v1alpha2.GatheringModeCustom,
				Custom: &v1alpha2.Custom{
					Configs: []v1alpha2.GathererConfig{
						{
							Name:  "mock_gatherer",
							State: v1alpha2.GathererStateDisabled,
						},
						{
							Name:  "foo_gatherer",
							State: v1alpha2.GathererStateDisabled,
						},
					},
				},
			},
		},
		{
			name: "Custom period gatherer is excluded because it should not be processed",
			gatherConfig: configv1alpha2.GatherConfig{
				DataPolicy: nil,
				Gatherers: configv1alpha2.Gatherers{
					Mode: configv1alpha2.GatheringModeCustom,
					Custom: &configv1alpha2.Custom{
						Configs: []configv1alpha2.GathererConfig{
							{
								Name:  "clusterconfig/bar",
								State: configv1alpha2.GathererStateDisabled,
							},
							{
								Name:  "mock_custom_period_gatherer_no_period",
								State: configv1alpha2.GathererStateDisabled,
							},
						},
					},
				},
				Storage: nil,
			},
			gatheres: []gatherers.Interface{
				&gather.MockGatherer{},
				&gather.MockCustomPeriodGathererNoPeriod{ShouldBeProcessed: false},
			},
			expectedPolicy: nil,
			expectedDisabledGatherers: v1alpha2.Gatherers{
				Mode: v1alpha2.GatheringModeCustom,
				Custom: &v1alpha2.Custom{
					Configs: []v1alpha2.GathererConfig{
						{
							Name:  "clusterconfig/bar",
							State: v1alpha2.GathererStateDisabled,
						},
						{
							Name:  "mock_custom_period_gatherer_no_period",
							State: v1alpha2.GathererStateDisabled,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPIConfig := config.NewMockAPIConfigurator(&tt.gatherConfig)
			mockController := NewWithTechPreview(nil, nil, mockAPIConfig, tt.gatheres, nil, nil, nil, nil, nil)
			disabledGatherers, dp, storage := mockController.createDataGatherAttributeValues()
			assert.Equal(t, tt.expectedPolicy, dp)
			assert.EqualValues(t, tt.expectedDisabledGatherers, disabledGatherers)
			assert.Equal(t, createStorage(tt.gatherConfig.Storage), storage)
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
			mockController := NewWithTechPreview(nil, nil, nil, nil, cs, nil, nil, nil, nil)
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
				&v1alpha2.DataGather{
					ObjectMeta: metav1.ObjectMeta{
						Name: "to-be-removed-dg-1",
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-25 * time.Hour),
						},
					},
				},
				&v1alpha2.DataGather{
					ObjectMeta: metav1.ObjectMeta{
						Name: "to-be-removed-dg-2",
						CreationTimestamp: metav1.Time{
							Time: metav1.Now().Time.Add(-1441 * time.Minute),
						},
					},
				},
				&v1alpha2.DataGather{
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
			mockController := NewWithTechPreview(nil, nil, nil, nil, kubeCs, insightsCs.InsightsV1alpha2(), nil, nil, nil)
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
			dataGathersList, err := insightsCs.InsightsV1alpha2().DataGathers().List(context.Background(), metav1.ListOptions{})
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
		testedDataGather *v1alpha2.DataGather
		expectedSummary  controllerstatus.Summary
	}{
		{
			name: "Data gather was successful",
			testedDataGather: &v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
							Reason: "Succeeded",
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
			testedDataGather: &v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
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
			testedDataGather: &v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-dg",
				},
				Status: v1alpha2.DataGatherStatus{
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
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
		dataGather        *v1alpha2.DataGather
		expectedProcessed bool
	}{
		{
			name: "Empty conditions - not processed",
			dataGather: &v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expectedProcessed: false,
		},
		{
			name: "DataProcessed status unknown - not processed",
			dataGather: &v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						status.DataProcessedCondition(metav1.ConditionUnknown, status.NothingToProcessYetReason, ""),
					},
				},
			},
			expectedProcessed: false,
		},
		{
			name: "DataProcessed status true - processed",
			dataGather: &v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
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
		dataGatherToUpdate     *v1alpha2.DataGather
		analysisReport         *types.InsightsAnalysisReport
		expectedInsightsReport *v1alpha2.InsightsReport
	}{
		{
			name: "DataGather is updated with active recommendations",
			dataGatherToUpdate: &v1alpha2.DataGather{
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
			expectedInsightsReport: &v1alpha2.InsightsReport{
				DownloadedTime: ptr.To(metav1.Date(2022, 11, 24, 5, 40, 0, 0, time.UTC)),
				URI:            "https://test.report.endpoint.tech.preview.uri/cluster/test-cluster-id/requestID/test-request-id",
				HealthChecks: []v1alpha2.HealthCheck{
					{
						Description: "lorem-ipsum",
						TotalRisk:   v1alpha2.TotalRiskLow,
						AdvisorURI:  "https://console.redhat.com/openshift/insights/advisor/clusters/test-cluster-id?first=test.fqdn.key1%7Ctest-error-key-1",
					},
					{
						Description: "lorem-ipsum bla bla test",
						TotalRisk:   v1alpha2.TotalRiskCritical,
						AdvisorURI:  "https://console.redhat.com/openshift/insights/advisor/clusters/test-cluster-id?first=test.fqdn.key2%7Ctest-error-key-2",
					},
				},
			},
		},
		{
			name: "No active recommendations",
			dataGatherToUpdate: &v1alpha2.DataGather{
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
			expectedInsightsReport: &v1alpha2.InsightsReport{
				DownloadedTime: ptr.To(metav1.Date(2022, 11, 24, 5, 40, 0, 0, time.UTC)),
				URI:            "https://test.report.endpoint.tech.preview.uri/cluster/test-cluster-id/requestID/test-request-id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insightsCs := insightsFakeCli.NewSimpleClientset(tt.dataGatherToUpdate)
			conf := &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					DownloadEndpointTechPreview: "https://test.report.endpoint.tech.preview.uri/cluster/%s/requestID/%s",
				},
			}
			mockCMConf := config.NewMockConfigMapConfigurator(conf)
			mockController := NewWithTechPreview(nil, mockCMConf, nil, nil, nil, insightsCs.InsightsV1alpha2(), nil, nil, nil)
			err := mockController.updateInsightsReportInDataGather(context.Background(), tt.analysisReport, tt.dataGatherToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedInsightsReport, &tt.dataGatherToUpdate.Status.InsightsReport)
		})
	}
}

func TestUpdateClusterOperatorConditions(t *testing.T) {
	tests := []struct {
		name               string
		dataGatherCR       v1alpha2.DataGather
		insightsClusterOp  configv1.ClusterOperator
		expectedConditions []configv1.ClusterOperatorStatusCondition
		expectedErr        error
	}{
		{
			name: "remote config conditions are unknown and should be updated",
			dataGatherCR: v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(status.RemoteConfigurationValid),
							Status: metav1.ConditionTrue,
						},
						{
							Type:    string(status.RemoteConfigurationAvailable),
							Status:  metav1.ConditionFalse,
							Reason:  "TestReason",
							Message: "This is a test error message",
						},
					},
				},
			},
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
						},
						{
							Type:   status.RemoteConfigurationAvailable,
							Status: configv1.ConditionUnknown,
						},
						{
							Type:   status.RemoteConfigurationValid,
							Status: configv1.ConditionUnknown,
						},
					},
				},
			},
			expectedConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:    status.RemoteConfigurationAvailable,
					Status:  configv1.ConditionFalse,
					Reason:  "TestReason",
					Message: "This is a test error message",
				},
				{
					Type:   status.RemoteConfigurationValid,
					Status: configv1.ConditionTrue,
				},
			},
			expectedErr: nil,
		},
		{
			name: "remote config condition does not exist in the DataGather CR",
			dataGatherCR: v1alpha2.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "periodic-test",
				},
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataRecorded,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
						},
						{
							Type:   status.RemoteConfigurationAvailable,
							Status: configv1.ConditionUnknown,
						},
					},
				},
			},
			expectedConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   status.RemoteConfigurationAvailable,
					Status: configv1.ConditionUnknown,
				},
			},
			expectedErr: fmt.Errorf("RemoteConfigurationAvailable condition not found in status of periodic-test dataGather"),
		},
		{
			name: "remote config condition does not exist in the ClusterOperator CR",
			dataGatherCR: v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(status.RemoteConfigurationAvailable),
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
						{
							Type:   string(status.RemoteConfigurationValid),
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
					},
				},
			},
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
						},
					},
				},
			},
			expectedConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   status.RemoteConfigurationAvailable,
					Status: configv1.ConditionTrue,
					Reason: "AsExpected",
				},
				{
					Type:   status.RemoteConfigurationValid,
					Status: configv1.ConditionTrue,
					Reason: "AsExpected",
				},
			},
		},
		{
			name: "remote config condition in ClusterOperator CR has the same status as in DataGather CR",
			dataGatherCR: v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(status.RemoteConfigurationAvailable),
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
						{
							Type:   string(status.RemoteConfigurationValid),
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
					},
				},
			},
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
						},
						{
							Type:   status.RemoteConfigurationAvailable,
							Status: configv1.ConditionTrue,
							Reason: "AsExpected",
						},
						{
							Type:   status.RemoteConfigurationValid,
							Status: configv1.ConditionTrue,
							Reason: "AsExpected",
						},
					},
				},
			},
			expectedConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   status.RemoteConfigurationAvailable,
					Status: configv1.ConditionTrue,
					Reason: "AsExpected",
				},
				{
					Type:   status.RemoteConfigurationValid,
					Status: configv1.ConditionTrue,
					Reason: "AsExpected",
				},
			},
		},
		{
			name: "remote config condition are True and should be updated to False status",
			dataGatherCR: v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   string(status.RemoteConfigurationAvailable),
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
						{
							Type:   string(status.RemoteConfigurationValid),
							Status: metav1.ConditionTrue,
							Reason: "AsExpected",
						},
					},
				},
			},
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
						},
						{
							Type:    status.RemoteConfigurationAvailable,
							Status:  configv1.ConditionFalse,
							Reason:  "NotAvailable",
							Message: "This is a unvailable error message",
						},
						{
							Type:    status.RemoteConfigurationValid,
							Status:  configv1.ConditionFalse,
							Reason:  "Invalid",
							Message: "This is a invalid error message",
						},
					},
				},
			},
			expectedConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   status.RemoteConfigurationAvailable,
					Status: configv1.ConditionTrue,
					Reason: "AsExpected",
				},
				{
					Type:   status.RemoteConfigurationValid,
					Status: configv1.ConditionTrue,
					Reason: "AsExpected",
				},
			},
		},
		{
			name: "remote config condition status is the same, but the reason is different",
			dataGatherCR: v1alpha2.DataGather{
				Status: v1alpha2.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:   status.DataUploaded,
							Status: metav1.ConditionTrue,
						},
						{
							Type:    string(status.RemoteConfigurationAvailable),
							Status:  metav1.ConditionFalse,
							Reason:  "NonHttp200Response",
							Message: "Receive HTTP 404 response",
						},
						{
							Type:    string(status.RemoteConfigurationValid),
							Status:  metav1.ConditionFalse,
							Reason:  "Invalid",
							Message: "Validation failed",
						},
					},
				},
			},
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
						},
						{
							Type:    status.RemoteConfigurationAvailable,
							Status:  configv1.ConditionFalse,
							Reason:  "NotAvailable",
							Message: "Cannot connect",
						},
						{
							Type:    status.RemoteConfigurationValid,
							Status:  configv1.ConditionFalse,
							Reason:  "Unknown",
							Message: "Cannot pass validation",
						},
					},
				},
			},
			expectedConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:    status.RemoteConfigurationAvailable,
					Status:  configv1.ConditionFalse,
					Reason:  "NonHttp200Response",
					Message: "Receive HTTP 404 response",
				},
				{
					Type:    status.RemoteConfigurationValid,
					Status:  configv1.ConditionFalse,
					Reason:  "Invalid",
					Message: "Validation failed",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			configCli := configFakeCli.NewSimpleClientset(&tt.insightsClusterOp)
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, nil, nil, configCli.ConfigV1(), nil)
			err := mockController.updateStatusBasedOnDataGatherCondition(ctx, &tt.dataGatherCR)
			assert.Equal(t, tt.expectedErr, err)
			insightsCO, err := configCli.ConfigV1().ClusterOperators().Get(ctx, "insights", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedConditions, insightsCO.Status.Conditions)
		})
	}
}

func TestSetRemoteConfigConditionsWhenDisabled(t *testing.T) {
	expectedRemoteConfigurationAvailable := configv1.ClusterOperatorStatusCondition{
		Type:    status.RemoteConfigurationAvailable,
		Status:  configv1.ConditionFalse,
		Reason:  gatheringDisabledReason,
		Message: "Data gathering is disabled",
	}
	expectedRemoteConfigurationValid := configv1.ClusterOperatorStatusCondition{
		Type:   status.RemoteConfigurationValid,
		Status: configv1.ConditionUnknown,
		Reason: status.RemoteConfNotValidatedYet,
	}

	tests := []struct {
		name                              string
		insightsClusterOp                 configv1.ClusterOperator
		expectedLenghtOfUpdatedConditions int
	}{
		{
			name: "Insights clusteroperator conditions are empty",
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{},
			},
			expectedLenghtOfUpdatedConditions: 2,
		},
		{
			name: `Insights clusteroperator conditions are not empty, 
			but remote configuration conditions don't exist`,
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:    configv1.OperatorAvailable,
							Status:  configv1.ConditionTrue,
							Reason:  "AsExpected",
							Message: "",
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
							Reason: "AsExpected",
						},
					},
				},
			},
			expectedLenghtOfUpdatedConditions: 4,
		},
		{
			name: `Remote Configuration conditions are updated as expected`,
			insightsClusterOp: configv1.ClusterOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "insights",
				},
				Status: configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:    configv1.OperatorAvailable,
							Status:  configv1.ConditionTrue,
							Reason:  "AsExpected",
							Message: "",
						},
						{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionFalse,
							Reason: "AsExpected",
						},
						{
							Type:    status.RemoteConfigurationAvailable,
							Status:  configv1.ConditionTrue,
							Reason:  "AsExpected",
							Message: "",
						},
						{
							Type:   status.RemoteConfigurationValid,
							Status: configv1.ConditionTrue,
							Reason: "AsExpected",
						},
					},
				},
			},
			expectedLenghtOfUpdatedConditions: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			configCli := configFakeCli.NewSimpleClientset(&tt.insightsClusterOp)
			mockController := NewWithTechPreview(nil, nil, nil, nil, nil, nil, nil, configCli.ConfigV1(), nil)
			err := mockController.setRemoteConfigConditionsWhenDisabled(ctx)
			assert.NoError(t, err)

			insightsCO, err := configCli.ConfigV1().ClusterOperators().Get(ctx, "insights", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Len(t, insightsCO.Status.Conditions, tt.expectedLenghtOfUpdatedConditions)
			actualRCA := getConditionByType(insightsCO.Status.Conditions, status.RemoteConfigurationAvailable)
			assert.True(t, areConditionsSameExceptTransitionTime(actualRCA, &expectedRemoteConfigurationAvailable))
			actualRCV := getConditionByType(insightsCO.Status.Conditions, status.RemoteConfigurationValid)
			assert.True(t, areConditionsSameExceptTransitionTime(actualRCV, &expectedRemoteConfigurationValid))
		})
	}
}

func areConditionsSameExceptTransitionTime(a, b *configv1.ClusterOperatorStatusCondition) bool {
	if a.Type != b.Type {
		return false
	}

	if a.Status != b.Status {
		return false
	}

	if a.Reason != b.Reason {
		return false
	}

	if a.Message != b.Message {
		return false
	}
	return true
}

func getConditionByType(conditions []configv1.ClusterOperatorStatusCondition,
	ctype configv1.ClusterStatusConditionType,
) *configv1.ClusterOperatorStatusCondition {
	for _, c := range conditions {
		if c.Type == ctype {
			return &c
		}
	}
	return nil
}
