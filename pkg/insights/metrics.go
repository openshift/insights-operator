package insights

import (
	"fmt"
	"strings"

	semver "github.com/blang/semver/v4"
	v1 "github.com/openshift/api/config/v1"
	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

var (
	RecommendationCollector = &InsightsRecommendationCollector{
		metricName: "insights_recommendation_active",
	}
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
	metricName            string
	clusterID             v1.ClusterID
}

func (c *InsightsRecommendationCollector) SetClusterID(clusterID v1.ClusterID) {
	c.clusterID = clusterID
}

func (c *InsightsRecommendationCollector) ClusterID() v1.ClusterID {
	return c.clusterID
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
		// There is ".report" at the end of the rule ID for some reason, which
		// should not be inserted into the URL.
		ruleIDStr = strings.TrimSuffix(ruleIDStr, ".report")
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(c.metricName, "", []string{}, prometheus.Labels{
				"description": rec.Description,
				"total_risk":  totalRiskToStr(rec.TotalRisk),
				"info_link":   fmt.Sprintf("https://console.redhat.com/openshift/insights/advisor/clusters/%s?first=%s|%s", c.clusterID, ruleIDStr, rec.ErrorKey),
			}),
			prometheus.GaugeValue,
			1,
		)
	}
}

func (c *InsightsRecommendationCollector) ClearState() {
	// NOOP: There is no state that would need to be cleared.
	// This method is implemented exclusively to comply with the Collector
	// interface from the legacyregistry module.
}

func (c *InsightsRecommendationCollector) Create(version *semver.Version) bool {
	return true
	// NOOP: No versioning is implemented for this collector.
	// This method is implemented exclusively to comply with the Collector
	// interface from the legacyregistry module.
}

func (c *InsightsRecommendationCollector) FQName() string {
	return c.metricName
}

func totalRiskToStr(totalRisk int) string {
	switch totalRisk {
	case 1:
		return "Low"
	case 2:
		return "Moderate"
	case 3:
		return "Important"
	case 4:
		return "Critical"
	default:
		return "Invalid"
	}
}
