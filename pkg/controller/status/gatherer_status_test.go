package status

import (
	"testing"
	"time"

	"github.com/openshift/api/insights/v1alpha1"
	v1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			gathererStatus := CreateOperatorGathererStatus(&tt.gfr)
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

func TestDataGatherStatusToOperatorStatus(t *testing.T) {
	tests := []struct {
		name                   string
		dataGather             v1alpha1.DataGather
		expectedOperatorstatus v1.InsightsOperatorStatus
	}{
		{
			name: "basic copy test",
			dataGather: v1alpha1.DataGather{
				Status: v1alpha1.DataGatherStatus{
					Conditions: []metav1.Condition{
						DataProcessedCondition(metav1.ConditionTrue, "EveyrthingOK", "no message"),
					},
					State:      v1alpha1.Completed,
					StartTime:  metav1.Date(2023, 7, 31, 5, 40, 15, 0, time.UTC),
					FinishTime: metav1.Date(2023, 7, 31, 5, 41, 04, 0, time.UTC),
					Gatherers: []v1alpha1.GathererStatus{
						{
							Name:       "test-gatherer-1",
							Conditions: []metav1.Condition{},
							LastGatherDuration: metav1.Duration{
								Duration: 94 * time.Second,
							},
						},
					},
				},
			},
			expectedOperatorstatus: v1.InsightsOperatorStatus{
				GatherStatus: v1.GatherStatus{
					LastGatherTime: metav1.Date(2023, 7, 31, 5, 41, 04, 0, time.UTC),
					LastGatherDuration: metav1.Duration{
						Duration: 49 * time.Second,
					},
					Gatherers: []v1.GathererStatus{
						{
							Name:       "test-gatherer-1",
							Conditions: []metav1.Condition{},
							LastGatherDuration: metav1.Duration{
								Duration: 94 * time.Second,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operatorStatus := DataGatherStatusToOperatorStatus(&tt.dataGather)
			assert.Equal(t, tt.expectedOperatorstatus, operatorStatus)
		})
	}
}
