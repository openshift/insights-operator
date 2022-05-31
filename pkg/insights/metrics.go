package insights

import (
	"fmt"

	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

var (
	RecommendationCollector = &InsightsRecommendationCollector{}
)

// MustRegisterMetrics registers provided registrables in the Insights metrics registry.
// This function should be called from init() functions only, because
// it uses the MustRegister method, and therefore panics in case of an error.
func MustRegisterMetrics(registrables ...metrics.Registerable) {
	for _, r := range registrables {
		err := legacyregistry.Register(r)
		if err != nil {
			klog.Errorf("Failed to register metric %s: %v", r.FQName(), err)
		}
	}
}

func init() {
	MustRegisterMetrics(RecommendationCollector)
}

type InsightsRecommendationCollector struct {
	activeRecommendations []types.InsightsRecommendation
}

func (c *InsightsRecommendationCollector) SetActiveRecommendations(activeRecommendations []types.InsightsRecommendation) {
	c.activeRecommendations = activeRecommendations
}

func (c *InsightsRecommendationCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *InsightsRecommendationCollector) Collect(ch chan<- prometheus.Metric) {
	for _, rec := range c.activeRecommendations {
		ruleIDStr := string(rec.RuleID)
		errorKeyStr := string(rec.ErrorKey)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("insights_recommendation_active", "", []string{}, prometheus.Labels{
				"rule_id":     ruleIDStr,
				"error_key":   errorKeyStr,
				"description": rec.Description,
				"total_risk":  totalRiskToStr(rec.TotalRisk),
				"info_link":   fmt.Sprintf("https://console.redhat.com/openshift/insights/advisor/recommendations/%s%%7C%s", ruleIDStr, errorKeyStr),
			}),
			prometheus.GaugeValue,
			1,
		)
	}
}

func totalRiskToStr(totalRisk int) string {
	switch totalRisk {
	case 1:
		return "low"
	case 2:
		return "moderate"
	case 3:
		return "important"
	case 4:
		return "critical"
	default:
		return "invalid"
	}
}
