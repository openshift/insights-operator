package insights

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/config"
	fakeMonCli "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckAlertsDisabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		promRulesExist bool
		alertsDisabled bool
	}{
		{
			name:           "Create Prometheus rule when alerts enabled and the rules don't exist ",
			promRulesExist: false,
			alertsDisabled: false,
		},
		{
			name:           "Create Prometheus rule when alerts disabled and the rules already exist",
			promRulesExist: true,
			alertsDisabled: false,
		},
		{
			name:           "Create Prometheus rule when alerts disabled and the rules don't exist",
			promRulesExist: false,
			alertsDisabled: true,
		},
		{
			name:           "Create Prometheus rule when alerts disabled and the rules already exist",
			promRulesExist: true,
			alertsDisabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigObserver := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
				Alerting: config.Alerting{
					Disabled: tt.alertsDisabled,
				},
			})
			testMonCli := fakeMonCli.NewSimpleClientset()
			mockPromController := PrometheusRulesController{
				configurator:   mockConfigObserver,
				promRulesExist: tt.promRulesExist,
				monitoringCS:   testMonCli,
			}
			if tt.promRulesExist {
				err := mockPromController.createInsightsAlerts(ctx)
				assert.NoError(t, err)
			}
			mockPromController.checkAlertsDisabled(ctx)

			insightsPromRule, err := testMonCli.MonitoringV1().PrometheusRules(namespaceName).Get(ctx, rulesName, metav1.GetOptions{})
			if tt.alertsDisabled {
				// prometheus rules are not created
				if tt.promRulesExist {
					assert.Equal(t, false, mockPromController.promRulesExist)
				}
				assert.Error(t, err)
				assert.Nil(t, insightsPromRule)
			} else {
				// prometheus rules are created
				assert.NoError(t, err)
				assert.True(t, mockPromController.promRulesExist)
				assert.NotEmpty(t, insightsPromRule)

				// check the defined group
				groups := insightsPromRule.Spec.Groups
				assert.Len(t, groups, 1)
				rules := insightsPromRule.Spec.Groups[0].Rules
				insightsDisabledRule := rules[0]
				scaNotAvailable := rules[1]
				insightsRecommendationActive := rules[2]

				assert.Equal(t, insightsDisabledAlert, insightsDisabledRule.Alert)
				assert.Equal(t, simpleContentAccessNotAvailableAlert, scaNotAvailable.Alert)
				assert.Equal(t, insightsRecommendationActiveAlert, insightsRecommendationActive.Alert)

				err = mockPromController.removeInsightsAlerts(ctx)
				assert.NoError(t, err)
			}
		})
	}

}
