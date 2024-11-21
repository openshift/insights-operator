package insights

import (
	"context"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringcli "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	rulesName      = "insights-prometheus-rules"
	namespaceName  = "openshift-insights"
	durationString = "5m"
	info           = "info"

	insightsDisabledAlert                = "InsightsDisabled"
	simpleContentAccessNotAvailableAlert = "SimpleContentAccessNotAvailable"
	insightsRecommendationActiveAlert    = "InsightsRecommendationActive"
)

// PrometheusRulesController listens to the configuration observer and
// creates or removes the Insights Prometheus Rules definitions accordingly
type PrometheusRulesController struct {
	configurator   configobserver.Interface
	monitoringCS   monitoringcli.Interface
	promRulesExist bool
}

func NewPrometheusRulesController(configurator configobserver.Interface, kubeConfig *rest.Config) PrometheusRulesController {
	monitoringCS, err := monitoringcli.NewForConfig(kubeConfig)
	if err != nil {
		klog.Warningf("Unable create monitoring client: %v", err)
	}
	return PrometheusRulesController{
		configurator: configurator,
		monitoringCS: monitoringCS,
	}
}

// Start starts listening to the configuration observer
func (p *PrometheusRulesController) Start(ctx context.Context) {
	configCh, cancel := p.configurator.ConfigChanged()
	defer cancel()

	p.checkAlertsDisabled(ctx)
	for {
		select {
		case <-configCh:
			p.checkAlertsDisabled(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// checkAlertsDisabled reads the actual config and either creates (if they don't exist) or removes (if they do exist)
// the "insights-prometheus-rules" definition
func (p *PrometheusRulesController) checkAlertsDisabled(ctx context.Context) {
	disableInsightsAlerts := p.configurator.Config().Alerting.Disabled

	if disableInsightsAlerts && p.promRulesExist {
		err := p.removeInsightsAlerts(ctx)
		if err != nil {
			klog.Errorf("Failed to remove Insights Prometheus rules definition: %v", err)
			return
		}
		klog.Info("Prometheus rules successfully removed")
		p.promRulesExist = false
	}

	if !disableInsightsAlerts && !p.promRulesExist {
		err := p.createInsightsAlerts(ctx)
		if err != nil {
			if errors.IsAlreadyExists(err) {
				p.promRulesExist = true
				return
			}
			klog.Errorf("Failed to create Insights Prometheus rules definition: %v", err)
			return
		}
		klog.Info("Prometheus rules successfully created")
		p.promRulesExist = true
	}
}

// createInsightsAlerts creates Insights Prometheus Rules definitions (including alerts)
func (p *PrometheusRulesController) createInsightsAlerts(ctx context.Context) error {

	forDuration := monitoringv1.Duration(durationString)

	pr := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rulesName,
			Namespace: namespaceName,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: "insights",
					Rules: []monitoringv1.Rule{
						{
							Alert: insightsDisabledAlert,
							Expr:  intstr.FromString("max without (job, pod, service, instance) (cluster_operator_conditions{name=\"insights\", condition=\"Disabled\"} == 1)"),
							For:   &forDuration,
							Labels: map[string]string{
								"severity":  info,
								"namespace": namespaceName,
							},
							Annotations: map[string]string{
								"description": "Insights operator is disabled. In order to enable Insights and benefit from recommendations specific to your cluster, please follow steps listed in the documentation: https://docs.openshift.com/container-platform/latest/support/remote_health_monitoring/enabling-remote-health-reporting.html",
								"summary":     "Insights operator is disabled.",
							},
						},
						{
							Alert: simpleContentAccessNotAvailableAlert,
							Expr:  intstr.FromString(" max without (job, pod, service, instance) (max_over_time(cluster_operator_conditions{name=\"insights\", condition=\"SCAAvailable\", reason=\"NotFound\"}[5m]) == 0)"),
							For:   &forDuration,
							Labels: map[string]string{
								"severity":  info,
								"namespace": namespaceName,
							},
							Annotations: map[string]string{
								"description": "Simple content access (SCA) is not enabled. Once enabled, Insights Operator can automatically import the SCA certificates from Red Hat OpenShift Cluster Manager making it easier to use the content provided by your Red Hat subscriptions when creating container images. See https://docs.openshift.com/container-platform/latest/cicd/builds/running-entitled-builds.html for more information.",
								"summary":     "Simple content access certificates are not available.",
							},
						},
						{
							Alert: insightsRecommendationActiveAlert,
							Expr:  intstr.FromString("insights_recommendation_active == 1"),
							For:   &forDuration,
							Labels: map[string]string{
								"severity": info,
							},
							Annotations: map[string]string{
								"description": "Insights recommendation \"{{ $labels.description }}\" with total risk \"{{ $labels.total_risk }}\" was detected on the cluster. More information is available at {{ $labels.info_link }}.",
								"summary":     "An Insights recommendation is active for this cluster.",
							},
						},
					},
				},
			},
		},
	}

	_, err := p.monitoringCS.MonitoringV1().PrometheusRules(namespaceName).Create(ctx, pr, metav1.CreateOptions{})
	return err
}

// removeInsightsAlerts removes the "insights-prometheus-rules" definition
func (p *PrometheusRulesController) removeInsightsAlerts(ctx context.Context) error {
	return p.monitoringCS.MonitoringV1().
		PrometheusRules(namespaceName).
		Delete(ctx, rulesName, metav1.DeleteOptions{})
}
