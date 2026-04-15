package retry

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
)

func TestShouldRetry_RetryOn500HTTP_Valid(t *testing.T) {
	// HTTP 500 error should trigger retry
	err := insightsclient.HttpError{
		StatusCode: http.StatusInternalServerError,
		Err:        fmt.Errorf("internal server error"),
	}

	result := shouldRetry(err, RetryOn500HTTP)
	if !result {
		t.Errorf("Expected shouldRetry to return true for HTTP 500 error, got false")
	}
}

func TestShouldRetry_RetryOn500HTTP_Invalid(t *testing.T) {
	// Test cases where RetryOn500HTTP should NOT retry:
	// 1. Non-HTTP error (network error, context error, etc.)
	// 2. HTTP 4xx error (client errors should not retry)

	// Test 1: Non-HTTP error
	err := fmt.Errorf("network connection error")
	result := shouldRetry(err, RetryOn500HTTP)
	if result {
		t.Errorf("Expected shouldRetry to return false for non-HTTP error, got true")
	}

	// Test 2: HTTP 4xx error
	err = insightsclient.HttpError{
		StatusCode: http.StatusNotFound,
		Err:        fmt.Errorf("not found"),
	}
	result = shouldRetry(err, RetryOn500HTTP)
	if result {
		t.Errorf("Expected shouldRetry to return false for HTTP 404 error, got true")
	}
}

func TestShouldRetry_RetryOnNon200HTTP_Valid(t *testing.T) {
	// Test cases where RetryOnNon200HTTP should retry:
	// 1. HTTP 404 error
	// 2. HTTP 500 error
	// 3. Non-HTTP error

	// Test 1: HTTP 404
	httpErr404 := insightsclient.HttpError{
		StatusCode: http.StatusNotFound,
		Err:        fmt.Errorf("not found"),
	}
	result := shouldRetry(httpErr404, RetryOnNon200HTTP)
	if !result {
		t.Errorf("Expected shouldRetry to return true for HTTP 404 error, got false")
	}

	// Test 2: HTTP 500 error
	httpErr500 := insightsclient.HttpError{
		StatusCode: http.StatusInternalServerError,
		Err:        fmt.Errorf("internal server error"),
	}
	result = shouldRetry(httpErr500, RetryOnNon200HTTP)
	if !result {
		t.Errorf("Expected shouldRetry to return true for HTTP 500 error, got false")
	}

	// Test 3: Non-HTTP error should retry
	nonHttpErr := fmt.Errorf("network error")
	result = shouldRetry(nonHttpErr, RetryOnNon200HTTP)
	if !result {
		t.Errorf("Expected shouldRetry to return true for non-HTTP error, got false")
	}
}

func TestShouldRetry_RetryOnNon200HTTP_Invalid(t *testing.T) {
	// HTTP 200 should NOT retry
	err := insightsclient.HttpError{
		StatusCode: http.StatusOK,
		Err:        nil,
	}
	result := shouldRetry(err, RetryOnNon200HTTP)
	if result {
		t.Errorf("Expected shouldRetry to return false for HTTP 200 response, got true")
	}
}

func TestShouldRetry_RetryOnAll_Valid(t *testing.T) {
	// All errors should trigger retry:
	// 1. HTTP error
	// 2. Non-HTTP error

	// Test 1: HTTP error
	httpErr := insightsclient.HttpError{
		StatusCode: http.StatusBadRequest,
		Err:        fmt.Errorf("bad request"),
	}
	result := shouldRetry(httpErr, RetryOnAll)
	if !result {
		t.Errorf("Expected shouldRetry to return true for HTTP 400 error, got false")
	}

	// Test 2: Non-HTTP error
	nonHttpErr := fmt.Errorf("some random error")
	result = shouldRetry(nonHttpErr, RetryOnAll)
	if !result {
		t.Errorf("Expected shouldRetry to return true for non-HTTP error, got false")
	}
}

func TestShouldRetry_RetryOnAll_Invalid(t *testing.T) {
	// For RetryOnAll, there's no "invalid" case where it shouldn't retry
	// But we can test that it returns true even for nil-ish cases or success responses

	// Even for HTTP 200 with error wrapper, it should retry
	err := insightsclient.HttpError{
		StatusCode: http.StatusOK,
		Err:        fmt.Errorf("unexpected error despite 200"),
	}
	result := shouldRetry(err, RetryOnAll)
	if !result {
		t.Errorf("Expected shouldRetry to return true for any error with RetryOnAll, got false")
	}
}

func TestShouldRetry_DefaultStrategy(t *testing.T) {
	// Invalid/unknown strategy should NOT retry
	invalidStrategy := RetryStrategy(999)
	err := fmt.Errorf("some error")

	result := shouldRetry(err, invalidStrategy)
	if result {
		t.Errorf("Expected shouldRetry to return false for unknown strategy, got true")
	}
}
