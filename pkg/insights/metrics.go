package insights

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var insightsMetricsRegistry *prometheus.Registry
var insightsCollector *insightsMetricsCollector

func init() {
	insightsMetricsRegistry = prometheus.NewRegistry()
	insightsCollector = &insightsMetricsCollector{}

	RegisterMetricCollectors(insightsCollector)
}

// RegisterMetricCollector registers a new metric collector or a new metric in
// the Insights metrics registry. This function should be called from init()
// functions only, because it uses the MustRegister method, and therefore panics
// in case of an error.
func RegisterMetricCollectors(collectors ...prometheus.Collector) {
	for _, c := range collectors {
		insightsMetricsRegistry.MustRegister(c)
	}
}

// Starts an HTTP Prometheus server for the Insights metrics registry.
func StartMetricsServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(insightsMetricsRegistry, promhttp.HandlerOpts{}))
	go http.ListenAndServe(":8080", mux)
}

type insightsMetricsCollector struct {
	exampleMetricValue int
}

func (c *insightsMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("example_insights_metric", "An example metric.", nil, nil)
}

func (c *insightsMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("example_insights_metric", "An example metric.", nil, nil),
		prometheus.GaugeValue,
		float64(c.exampleMetricValue),
	)
}
