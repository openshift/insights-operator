package periodic

import (
	"testing"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_eventHandler_addFunc(t *testing.T) {
	tests := []struct {
		name             string
		dataGather       insightsv1.DataGather
		expectChannelMsg bool
		expectedName     string
	}{
		{
			name: "non-periodic DataGather triggers channel message",
			dataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "on-demand-gather",
				},
			},
			expectChannelMsg: true,
			expectedName:     "on-demand-gather",
		},
		{
			name: "periodic DataGather is filtered out",
			dataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "periodic-gathering-12345",
				},
			},
			expectChannelMsg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataGatherController := &dataGatherController{
				ch:            make(chan string, 1),
				statusChanged: make(chan struct{}),
			}
			handler := dataGatherController.eventHandler()

			// Act
			handler.OnAdd(&tt.dataGather, false)

			// Assert
			if tt.expectChannelMsg {
				select {
				case msg := <-dataGatherController.ch:
					assert.Equal(t, tt.expectedName, msg,
						"expected channel message %q, got %q", tt.expectedName, msg,
					)
				default:
					assert.Fail(t, "expected channel message but got none")
				}
			} else {
				select {
				case msg := <-dataGatherController.ch:
					assert.Fail(t, "expected no channel message but got %q", msg)
				default:
					// Expected: no message
				}
			}
		})
	}
}

func Test_eventHandler_updateFunc(t *testing.T) {
	tests := []struct {
		name               string
		oldDataGather      insightsv1.DataGather
		newDataGather      insightsv1.DataGather
		expectStatusSignal bool
	}{
		{
			name: "status changed from running to succeeded triggers signal",
			oldDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionTrue,
							Reason:             status.GatheringReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			newDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionFalse,
							Reason:             status.GatheringSucceededReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectStatusSignal: true,
		},
		{
			name: "status changed from running to failed triggers signal",
			oldDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionTrue,
							Reason:             status.GatheringReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			newDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionFalse,
							Reason:             status.GatheringFailedReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectStatusSignal: true,
		},
		{
			name: "periodic DataGather update is filtered out",
			oldDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "periodic-gathering-12345",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionTrue,
							Reason:             status.GatheringReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			newDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "periodic-gathering-12345",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionFalse,
							Reason:             status.GatheringSucceededReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectStatusSignal: false,
		},
		{
			name: "no status change does not trigger signal",
			oldDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionTrue,
							Reason:             status.GatheringReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			newDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionTrue,
							Reason:             status.GatheringReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectStatusSignal: false,
		},
		{
			name: "update to non-finished does not trigger signal",
			oldDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionFalse,
							Reason:             status.DataGatheringPendingReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			newDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionTrue,
							Reason:             status.GatheringReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectStatusSignal: false,
		},
		{
			name: "old DataGather without condition does not trigger signal",
			oldDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{},
				},
			},
			newDataGather: insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gather",
				},
				Status: insightsv1.DataGatherStatus{
					Conditions: []metav1.Condition{
						{
							Type:               status.Progressing,
							Status:             metav1.ConditionFalse,
							Reason:             status.GatheringSucceededReason,
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			},
			expectStatusSignal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataGatherController := &dataGatherController{
				ch:            make(chan string),
				statusChanged: make(chan struct{}, 10),
			}
			handler := dataGatherController.eventHandler()

			// Act
			handler.OnUpdate(&tt.oldDataGather, &tt.newDataGather)

			// Assert
			if tt.expectStatusSignal {
				select {
				case <-dataGatherController.statusChanged:
					// Expected: signal received
				default:
					assert.Fail(t, "expected status change signal but got none")
				}
			} else {
				select {
				case <-dataGatherController.statusChanged:
					assert.Fail(t, "expected no status change signal but got one")
				default:
					// Expected: no signal
				}
			}
		})
	}
}
