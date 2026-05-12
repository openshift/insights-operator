package insights

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	v1 "github.com/openshift/api/config/v1"
	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
)

var (
	RecommendationCollector = &Collector{
		metricName: "insights_recommendation_active",
	}
	counterRequestSend = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "insightsclient_request_send_total",
		Help: "Tracks the number of archives sent",
	}, []string{"client", "status_code"})
)

// RegisterInsightsMetrics registers all insights-operator Prometheus metrics with the provided registry.
// Should be called before starting the metrics server.
func RegisterInsightsMetrics(registry prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		RecommendationCollector,
		counterRequestSend,
	}
	for _, c := range collectors {
		if err := registry.Register(c); err != nil {
			return fmt.Errorf("failed to register insights metric: %w", err)
		}
	}
	return nil
}

func IncrementCounterRequestSend(status int) {
	statusCodeString := strconv.Itoa(status)
	counterRequestSend.WithLabelValues("insights", statusCodeString).Inc()
}

// Collector collects insights recommendations
type Collector struct {
	activeRecommendations []types.InsightsRecommendation
	metricName            string
	clusterID             v1.ClusterID
}

func (c *Collector) SetClusterID(clusterID v1.ClusterID) {
	c.clusterID = clusterID
}

func (c *Collector) ClusterID() v1.ClusterID {
	return c.clusterID
}

func (c *Collector) SetActiveRecommendations(activeRecommendations []types.InsightsRecommendation) {
	c.activeRecommendations = activeRecommendations
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	for _, rec := range c.activeRecommendations {
		ruleIDStr := string(rec.RuleID)
		// There is ".report" at the end of the rule ID for some reason, which
		// should not be inserted into the URL.
		ruleIDStr = strings.TrimSuffix(ruleIDStr, ".report")

		consoleURL, err := CreateInsightsAdvisorLink(c.clusterID, ruleIDStr, rec.ErrorKey)
		if err != nil {
			klog.Errorf("Failed to create console.redhat.com link: %v", err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(c.metricName, "", []string{}, prometheus.Labels{
				"description": rec.Description,
				"total_risk":  totalRiskToStr(rec.TotalRisk),
				"info_link":   consoleURL,
			}),
			prometheus.GaugeValue,
			1,
		)
	}
}

// createURL parses, creates and encodes all the necessary parameters for the Insights recommendation
// link to the Insights advisor
func CreateInsightsAdvisorLink(clusterID v1.ClusterID, ruleID, errorKey string) (string, error) {
	consoleURL, err := url.Parse("https://console.redhat.com/openshift/insights/advisor/clusters")
	if err != nil {
		return "", err
	}
	consoleURL = consoleURL.JoinPath(string(clusterID))
	params := url.Values{}
	params.Add("first", fmt.Sprintf("%s|%s", ruleID, errorKey))
	consoleURL.RawQuery = params.Encode()
	return consoleURL.String(), nil
}


func totalRiskToStr(totalRisk int32) string {
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
