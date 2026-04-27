// Package retry provides shared retry logic with exponential backoff for HTTP operations.
//
// Usage example:
//
//	result, err := retry.RetryWithExpBackOff(
//		ctx,
//		wait.Backoff{
//			Duration: interval/32,
//			Factor: 2,
//			Steps: ocm.FailureCountThreshold,
//			Cap: interval,
//		},
//		retry.RetryOn50xHTTP,
//		func() (retry.Result, error) {
//			data, err := client.RecvSCACerts(ctx, endpoint, nodeArchs)
//			return retry.Result{Data: data}, err
//		},
//	)
package retry

import (
	"context"
	"errors"
	"net/http"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

type RetryStrategy int64

const (
	// RetryOn50xHTTP retries only on HTTP 500+ errors, skips retry for non-HTTP errors (disconnected env)
	// Used by: sca.go, clustertransfer.go
	RetryOn50xHTTP RetryStrategy = iota

	// RetryOnNon200HTTP retries on any non-200 HTTP status code
	// Used by: conditional_gatherer.go
	RetryOnNon200HTTP

	// RetryOnAll retries on all errors
	// Used by: insightsuploader.go
	RetryOnAll
)

// Result holds the response data from retry operations
type Result struct {
	Data       []byte
	StatusCode int
	RequestID  string
}

// shouldRetry determines if an error should be retried based on the strategy.
// Returns true if retry should be attempted (when steps remain).
// Returns false immediately if the context is canceled or deadline exceeded.
func shouldRetry(ctx context.Context, err error, strategy RetryStrategy) bool {
	// Don't retry if context is canceled or deadline exceeded
	if ctx.Err() != nil {
		return false
	}

	// Don't retry context cancellation or deadline errors
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Extract status code from HttpError (handles both pointer and non-pointer)
	var statusCode int
	var isHTTPError bool

	switch e := err.(type) {
	case *insightsclient.HttpError:
		// Pointer - real-world case from newHTTPErrorFromResponse
		statusCode = e.StatusCode
		isHTTPError = true
	case insightsclient.HttpError:
		// Non-pointer - test case
		statusCode = e.StatusCode
		isHTTPError = true
	}

	switch strategy {
	case RetryOn50xHTTP:
		// Only retry HTTP 500+ errors, skip non-HTTP errors (disconnected env)
		if !isHTTPError {
			return false
		}
		return statusCode >= http.StatusInternalServerError

	case RetryOnNon200HTTP:
		// Retry on any non-200 HTTP status, or non-HTTP errors
		if !isHTTPError {
			return true // retry non-HTTP errors
		}
		return statusCode != http.StatusOK

	case RetryOnAll:
		// Retry on all errors
		return true

	default:
		// Unknown strategy, don't retry
		klog.Infof("Unknown strategy %d for retry mechanism", strategy)
		return false
	}
}

func RetryWithExpBackOff(ctx context.Context, bo wait.Backoff, strategy RetryStrategy, operation func() (Result, error)) (Result, error) {
	var lastErr error
	var result Result

	attempt := 0
	maxAttempts := bo.Steps

	err := wait.ExponentialBackoffWithContext(ctx, bo, func(context.Context) (bool, error) {
		attempt++
		result, lastErr = operation()
		if lastErr != nil {
			// Use strategy to determine if we should retry
			if shouldRetry(ctx, lastErr, strategy) {
				klog.Errorf("%v. Retrying (attempt %d/%d)", lastErr, attempt, maxAttempts)
				return false, nil
			}
			// Don't retry based on strategy
			return true, lastErr
		}

		return true, nil
	})

	// If we exhausted retries, return the last operation error instead of the timeout error
	if wait.Interrupted(err) && lastErr != nil {
		return result, lastErr
	}

	return result, err
}
