// Package retry provides shared retry logic with exponential backoff for HTTP operations.
//
// Usage example (once implemented):
//
//	response, err := retry.RetryWithExpBackOff(
//		wait.Backoff{
//			Duration: interval/32,
//			Factor: 2,
//			Steps: ocm.FailureCountThreshold,
//			Cap: interval,
//		},
//		retry.RetryOn500HTTP,
//		func() (*Response, error) {
//			return client.RecvSCACerts(ctx, endpoint, nodeArchs)
//		},
//	)
package retry

import (
	"net/http"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

type RetryStrategy int64

const (
	// RetryOn500HTTP retries only on HTTP 500+ errors, skips retry for non-HTTP errors (disconnected env)
	// Used by: sca.go, clustertransfer.go
	RetryOn500HTTP RetryStrategy = iota

	// RetryOnNon200HTTP retries on any non-200 HTTP status code
	// Used by: conditional_gatherer.go
	RetryOnNon200HTTP

	// RetryOnAll retries on all errors
	// Used by: insightsuploader.go
	RetryOnAll
)

// shouldRetry determines if an error should be retried based on the strategy.
// Returns true if retry should be attempted (when steps remain).
func shouldRetry(err error, strategy RetryStrategy) bool {
	switch strategy {
	case RetryOn500HTTP:
		// Only retry HTTP 500+ errors, skip non-HTTP errors (disconnected env)
		if !insightsclient.IsHttpError(err) {
			return false
		}
		httpErr := err.(insightsclient.HttpError)
		return httpErr.StatusCode >= http.StatusInternalServerError

	case RetryOnNon200HTTP:
		// Retry on any non-200 HTTP status, or non-HTTP errors
		if !insightsclient.IsHttpError(err) {
			return true // retry non-HTTP errors
		}
		httpErr := err.(insightsclient.HttpError)
		return httpErr.StatusCode != http.StatusOK

	case RetryOnAll:
		// Retry on all errors
		return true

	default:
		// Unknown strategy, don't retry
		klog.Infof("Unknown strategy %d", strategy)
		return false
	}
}

func RetryWithExpBackOff[T any](bo wait.Backoff, strategy RetryStrategy, operation func() (T, error)) (T, error) {
	var err error
	var data T

	err = wait.ExponentialBackoff(bo, func() (bool, error) {
		data, err = operation()
		if err != nil {
			// Use strategy to determine if we should retry
			if shouldRetry(err, strategy) && bo.Steps > 1 {
				klog.Errorf("%v. Trying again in %s", err, bo.Step())
				return false, nil
			}
			// Don't retry - either strategy says no, or no steps remaining
			return true, err
		}

		return true, nil
	})

	return data, err
}
