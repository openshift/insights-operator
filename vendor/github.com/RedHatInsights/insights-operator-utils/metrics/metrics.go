/*
Copyright © 2020 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package metrics contains all metrics that needs to be exposed to Prometheus
// and indirectly to Grafana. Currently, the following metrics are exposed:
//
// api_endpoints_requests - number of requests made for each REST API endpoint
//
// api_endpoints_response_time - response times for all REST API endpoints
//
// api_endpoints_status_codes - number of responses for each status code
//
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// APIRequests is a counter vector for requests to endpoints
	APIRequests *prometheus.CounterVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_endpoints_requests",
		Help: "The total number of requests per endpoint",
	}, []string{"endpoint"})

	// APIResponsesTime collects the information about api response time per endpoint
	APIResponsesTime *prometheus.HistogramVec = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "api_endpoints_response_time",
		Help:    "API endpoints response time",
		Buckets: prometheus.LinearBuckets(0, 20, 20),
	}, []string{"endpoint"})

	// APIResponseStatusCodes collects the information about api response status codes
	APIResponseStatusCodes *prometheus.CounterVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "api_endpoints_status_codes",
		Help: "API endpoints status codes",
	}, []string{"status_code"})
)

// AddAPIMetricsWithNamespace overwrite the defined metrics with namespaced version of them
func AddAPIMetricsWithNamespace(namespace string) {
	prometheus.Unregister(APIRequests)
	prometheus.Unregister(APIResponsesTime)
	prometheus.Unregister(APIResponseStatusCodes)

	APIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "api_endpoints_requests",
		Help:      "The total number of requests per endpoint",
	}, []string{"endpoint"})

	APIResponsesTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "api_endpoints_response_time",
		Help:      "API endpoints response time",
		Buckets:   prometheus.LinearBuckets(0, 20, 20),
	}, []string{"endpoint"})

	APIResponseStatusCodes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "api_endpoints_status_codes",
		Help:      "API endpoints status codes",
	}, []string{"status_code"})
}
