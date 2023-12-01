package status

import (
	"context"
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			klog.SetOutput(utils.NewTestLog(t).Writer())
			var operators []runtime.Object
			if tt.clusterOperator != nil {
				operators = append(operators, tt.clusterOperator)
			}

			client := configfake.NewSimpleClientset(operators...)
			ctrl := &Controller{
				name:   "insights",
				client: client.ConfigV1(),
				configurator: config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
					DataReporting: config.DataReporting{
						Enabled: true,
					},
				}),
				ctrlStatus: newControllerStatus(),
			}

			err := ctrl.updateStatus(context.Background(), tt.initialRun)
			if err != tt.expErr {
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
		Reason:             asExpectedReason,
		Message:            insightsAvailableMessage,
		LastTransitionTime: lastTransitionTime,
	}
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorProgressing,
		Status:             configv1.ConditionFalse,
		Reason:             asExpectedReason,
		Message:            monitoringMsg,
		LastTransitionTime: lastTransitionTime,
	}
	degradedCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorDegraded,
		Status:             configv1.ConditionFalse,
		Reason:             asExpectedReason,
		Message:            insightsAvailableMessage,
		LastTransitionTime: lastTransitionTime,
	}
	upgradeableCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorUpgradeable,
		Status:             configv1.ConditionTrue,
		Reason:             upgradeableReason,
		Message:            canBeUpgradedMsg,
		LastTransitionTime: lastTransitionTime,
	}

	testCO := configv1.ClusterOperator{
		Status: configv1.ClusterOperatorStatus{
			Conditions: []configv1.ClusterOperatorStatusCondition{
				availableCondition,
				progressingCondition,
				degradedCondition,
				upgradeableCondition,
				{
					Type:               OperatorDisabled,
					Status:             configv1.ConditionFalse,
					Reason:             asExpectedReason,
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
		apiConfigurator: config.NewMockAPIConfigurator(nil),
	}
	updatedCO := testController.merge(&testCO)
	// check that all the conditions are not touched except the disabled one
	assert.Equal(t, availableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable))
	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))
	assert.Equal(t, degradedCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded))
	assert.Equal(t, upgradeableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable))

	disabledCondition := getConditionByType(updatedCO.Status.Conditions, OperatorDisabled)
	assert.Equal(t, configv1.ConditionTrue, disabledCondition.Status)
	assert.Equal(t, noTokenReason, disabledCondition.Reason)
	assert.Equal(t, reportingDisabledMsg, disabledCondition.Message)
	assert.True(t, disabledCondition.LastTransitionTime.After(lastTransitionTime.Time))

	// upgrade status again and nothing should change
	updatedCO = testController.merge(updatedCO)
	// check that all the conditions are not touched including the disabled one
	assert.Equal(t, availableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable))
	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))
	assert.Equal(t, degradedCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded))
	assert.Equal(t, upgradeableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable))
	assert.Equal(t, disabledCondition, getConditionByType(updatedCO.Status.Conditions, OperatorDisabled))
}

func Test_updatingConditionsFromDegradedToDisabled(t *testing.T) {
	lastTransitionTime := metav1.Date(2022, 3, 21, 16, 20, 30, 0, time.UTC)
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type:               configv1.OperatorProgressing,
		Status:             configv1.ConditionFalse,
		Reason:             asExpectedReason,
		Message:            monitoringMsg,
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
					Type:               configv1.OperatorUpgradeable,
					Status:             configv1.ConditionFalse,
					Reason:             degradedReason,
					LastTransitionTime: lastTransitionTime,
				},
				{
					Type:               OperatorDisabled,
					Status:             configv1.ConditionFalse,
					Reason:             asExpectedReason,
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
		apiConfigurator: config.NewMockAPIConfigurator(nil),
	}
	updatedCO := testController.merge(&testCO)
	// check that all conditions changed except the Progressing since it's still False
	availableCondition := *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable)
	assert.Equal(t, availableCondition.Status, configv1.ConditionTrue)
	assert.True(t, availableCondition.LastTransitionTime.After(lastTransitionTime.Time))

	degradedCondition := *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded)
	assert.Equal(t, degradedCondition.Status, configv1.ConditionFalse)
	assert.True(t, degradedCondition.LastTransitionTime.After(lastTransitionTime.Time))

	upgradeableCondition := *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable)
	assert.Equal(t, upgradeableCondition.Status, configv1.ConditionTrue)
	assert.True(t, upgradeableCondition.LastTransitionTime.After(lastTransitionTime.Time))

	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))

	disabledCondition := getConditionByType(updatedCO.Status.Conditions, OperatorDisabled)
	assert.Equal(t, configv1.ConditionTrue, disabledCondition.Status)
	assert.Equal(t, noTokenReason, disabledCondition.Reason)
	assert.Equal(t, reportingDisabledMsg, disabledCondition.Message)
	assert.True(t, disabledCondition.LastTransitionTime.After(lastTransitionTime.Time))

	// upgrade status again and nothing should change
	updatedCO = testController.merge(updatedCO)
	// check that all the conditions are not touched including the disabled one
	assert.Equal(t, availableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorAvailable))
	assert.Equal(t, progressingCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorProgressing))
	assert.Equal(t, degradedCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorDegraded))
	assert.Equal(t, upgradeableCondition, *getConditionByType(updatedCO.Status.Conditions, configv1.OperatorUpgradeable))
	assert.Equal(t, disabledCondition, getConditionByType(updatedCO.Status.Conditions, OperatorDisabled))
}

func getConditionByType(conditions []configv1.ClusterOperatorStatusCondition,
	ctype configv1.ClusterStatusConditionType) *configv1.ClusterOperatorStatusCondition {
	for _, c := range conditions {
		if c.Type == ctype {
			return &c
		}
	}
	return nil
}
