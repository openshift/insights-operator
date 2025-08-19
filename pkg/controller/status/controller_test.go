package status

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Status_SaveInitialStart(t *testing.T) {
	tests := []struct {
		name                     string
		clusterOperator          *configv1.ClusterOperator
		expErr                   error
		initialRun               bool
		expectedSafeInitialStart bool
	}{
		{
			name:                     "Non-initial run has its upload delayed",
			initialRun:               false,
			expectedSafeInitialStart: false,
		},
		{
			name:                     "Initial run with not existing Insights operator is not delayed",
			initialRun:               true,
			clusterOperator:          nil,
			expectedSafeInitialStart: true,
		},
		{
			name:       "Initial run with existing Insights operator which is degraded is delayed",
			initialRun: true,
			clusterOperator: newClusterOperator(
				"insights",
				&configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
					{Type: configv1.OperatorDegraded, Status: configv1.ConditionTrue},
				}}),
			expectedSafeInitialStart: false,
		},
		{
			name:       "Initial run with existing Insights operator which is not degraded not delayed",
			initialRun: true,
			clusterOperator: newClusterOperator("insights",
				&configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
					{Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse},
				}}),
			expectedSafeInitialStart: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			klog.SetOutput(utils.NewTestLog(t).Writer())
			var operators []runtime.Object
			if tt.clusterOperator != nil {
				operators = append(operators, tt.clusterOperator)
			}

			client := configfake.NewSimpleClientset(operators...)
			mockAPIConfigurator := config.NewMockAPIConfigurator(
				&configv1.GatherConfig{
					Gatherers: configv1.Gatherers{
						Mode: configv1.GatheringModeNone,
					},
				},
			)
			ctrl := &Controller{
				name:   "insights",
				client: client.ConfigV1(),
				configurator: config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
					DataReporting: config.DataReporting{
						Enabled: true,
					},
				}),
				apiConfigurator: mockAPIConfigurator,
				ctrlStatus:      newControllerStatus(),
			}

			err := ctrl.updateStatus(context.Background(), tt.initialRun)
			if !errors.Is(err, tt.expErr) {
				t.Fatalf("updateStatus returned unexpected error: %s Expected %s", err, tt.expErr)
			}
		})
	}
}

func Test_updatingConditionsInDisabledState(t *testing.T) {
	lastTransitionTime := metav1.Date(2022, 3, 21, 16, 20, 30, 0, time.UTC)

	availableCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorAvailable,
		Status:             configv1.ConditionTrue,
		Reason:             AsExpectedReason,
		Message:            insightsAvailableMessage,
		LastTransitionTime: lastTransitionTime,
	}
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorProgressing,
		Status:             configv1.ConditionFalse,
		Reason:             AsExpectedReason,
		Message:            monitoringMessage,
		LastTransitionTime: lastTransitionTime,
	}
	degradedCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorDegraded,
		Status:             configv1.ConditionFalse,
		Reason:             AsExpectedReason,
		Message:            insightsAvailableMessage,
		LastTransitionTime: lastTransitionTime,
	}
	testCO := configv1.ClusterOperator{
		Status: configv1.ClusterOperatorStatus{
			Conditions: []configv1.ClusterOperatorStatusCondition{
				availableCondition,
				progressingCondition,
				degradedCondition,
				{
					Type:               OperatorDisabled,
					Status:             configv1.ConditionFalse,
					Reason:             AsExpectedReason,
					LastTransitionTime: lastTransitionTime,
				},
			},
		},
	}
	testController := Controller{
		ctrlStatus: newControllerStatus(),
		// marking operator as disabled
		configurator: config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
			DataReporting: config.DataReporting{
				Enabled: false,
			},
		}),
		apiConfigurator: config.NewMockAPIConfigurator(&configv1.GatherConfig{
			Gatherers: configv1.Gatherers{
				// Gathering enabled in configuration
				Mode: configv1.GatheringModeAll,
			},
		}),
	}
	updatedCO := testController.merge(&testCO)
	// check that all the conditions are not touched except the disabled one
	assert.Equal(t, availableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable))
	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))
	assert.Equal(t, degradedCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded))
	// Upgradeable should not  be set
	assert.Nil(t, getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable))

	disabledCondition := getConditionByType(updatedCO.Status.Conditions, OperatorDisabled)
	assert.Equal(t, configv1.ConditionTrue, disabledCondition.Status)
	assert.Equal(t, noTokenReason, disabledCondition.Reason)
	assert.Equal(t, reportingDisabledMessage, disabledCondition.Message)
	assert.True(t, disabledCondition.LastTransitionTime.After(lastTransitionTime.Time))

	// upgrade status again and nothing should change
	updatedCO = testController.merge(updatedCO)
	// check that all the conditions are not touched including the disabled one
	assert.Equal(t, availableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable))
	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))
	assert.Equal(t, degradedCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded))
	// Upgradeable should not  be set
	assert.Nil(t, getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable))
	assert.Equal(t, disabledCondition, getConditionByType(updatedCO.Status.Conditions, OperatorDisabled))
}

func Test_updatingConditionsFromDegradedToDisabled(t *testing.T) {
	lastTransitionTime := metav1.Date(2022, 3, 21, 16, 20, 30, 0, time.UTC)
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorProgressing,
		Status:             configv1.ConditionFalse,
		Reason:             AsExpectedReason,
		Message:            monitoringMessage,
		LastTransitionTime: lastTransitionTime,
	}
	testCO := configv1.ClusterOperator{
		Status: configv1.ClusterOperatorStatus{
			Conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionFalse,
					Reason:             "UploadFailed",
					LastTransitionTime: lastTransitionTime,
				},
				progressingCondition,
				{
					Type:               configv1.OperatorDegraded,
					Status:             configv1.ConditionTrue,
					Reason:             "UploadFailed",
					LastTransitionTime: lastTransitionTime,
				},
				{
					Type:               OperatorDisabled,
					Status:             configv1.ConditionFalse,
					Reason:             AsExpectedReason,
					LastTransitionTime: lastTransitionTime,
				},
			},
		},
	}
	testController := Controller{
		ctrlStatus: newControllerStatus(),
		// marking operator as disabled
		configurator: config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
			DataReporting: config.DataReporting{
				Enabled: false,
			},
		}),
		apiConfigurator: config.NewMockAPIConfigurator(&configv1.GatherConfig{
			Gatherers: configv1.Gatherers{
				Mode: configv1.GatheringModeAll,
			},
		}),
	}
	updatedCO := testController.merge(&testCO)
	// check that all conditions changed except the Progressing since it's still False
	availableCondition := *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable)
	assert.Equal(t, availableCondition.Status, configv1.ConditionTrue)
	assert.True(t, availableCondition.LastTransitionTime.After(lastTransitionTime.Time))

	degradedCondition := *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded)
	assert.Equal(t, degradedCondition.Status, configv1.ConditionFalse)
	assert.True(t, degradedCondition.LastTransitionTime.After(lastTransitionTime.Time))

	// Upgradeable should not be set
	assert.Nil(t, getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable))

	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))

	disabledCondition := getConditionByType(updatedCO.Status.Conditions, OperatorDisabled)
	assert.Equal(t, configv1.ConditionTrue, disabledCondition.Status)
	assert.Equal(t, noTokenReason, disabledCondition.Reason)
	assert.Equal(t, reportingDisabledMessage, disabledCondition.Message)
	assert.True(t, disabledCondition.LastTransitionTime.After(lastTransitionTime.Time))

	// upgrade status again and nothing should change
	updatedCO = testController.merge(updatedCO)
	// check that all the conditions are not touched including the disabled one
	assert.Equal(t, availableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable))
	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))
	assert.Equal(t, degradedCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded))
	assert.Equal(t, disabledCondition, getConditionByType(updatedCO.Status.Conditions, OperatorDisabled))
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
