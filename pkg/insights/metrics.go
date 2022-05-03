package insights

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
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
}

// RunMetricsServer starts an HTTP server for the Insights metrics registry.
// The server will run synchronously in an infinite loop. In case of an error,
// it will be logged, and the server will be restarted after a short sleep
// (to avoid spamming the log with the same error).
func RunMetricsServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(insightsMetricsRegistry, promhttp.HandlerOpts{}))
	for {
		klog.Errorf("Unable to serve metrics: %v", http.ListenAndServe(":8080", mux))
		time.Sleep(time.Minute)
	}
}

// RegisterMetricCollector registers a new metric collector or a new metric in
// the Insights metrics registry. This function should be called from init()
// functions only, because it uses the MustRegister method, and therefore panics
// in case of an error.
func MustRegisterMetricCollectors(collectors ...prometheus.Collector) {
	insightsMetricsRegistry.MustRegister(collectors...)
}
