package insights

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	insightsMetricsRegistry *prometheus.Registry
)

func init() {
	insightsMetricsRegistry = prometheus.NewRegistry()
}

// RegisterMetricCollector registers a new metric collector or a new metric in
// the Insights metrics registry. This function should be called from init()
// functions only, because it uses the MustRegister method, and therefore panics
// in case of an error.
func MustRegisterMetricCollectors(collectors ...prometheus.Collector) {
	insightsMetricsRegistry.MustRegister(collectors...)
}

// Starts an HTTP Prometheus server for the Insights metrics registry.
func StartMetricsServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(insightsMetricsRegistry, promhttp.HandlerOpts{}))
	go http.ListenAndServe(":8080", mux)
}
