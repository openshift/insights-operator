package insights

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	insightsMetricsRegistry *prometheus.Registry
)

func init() {
	insightsMetricsRegistry = prometheus.NewRegistry()
	MustRegisterMetricCollectors(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	startMetricsServer()
}

// startMetricsServer starts an HTTP server for the Insights metrics registry.
func startMetricsServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(insightsMetricsRegistry, promhttp.HandlerOpts{}))
	go http.ListenAndServe(":8080", mux)
}

// RegisterMetricCollector registers a new metric collector or a new metric in
// the Insights metrics registry. This function should be called from init()
// functions only, because it uses the MustRegister method, and therefore panics
// in case of an error.
func MustRegisterMetricCollectors(collectors ...prometheus.Collector) {
	insightsMetricsRegistry.MustRegister(collectors...)
}
