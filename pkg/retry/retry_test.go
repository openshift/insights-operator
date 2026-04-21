package retry

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"k8s.io/apimachinery/pkg/util/wait"
)

func Test_ShouldRetry(t *testing.T) {
	type testCase struct {
		name        string
		err         error
		strategy    RetryStrategy
		shouldRetry bool
	}

	testCases := []testCase{
		// RetryOn500HTTP strategy tests
		{
			name: "RetryOn500HTTP should retry on HTTP 500",
			err: insightsclient.HttpError{
				StatusCode: http.StatusInternalServerError,
				Err:        fmt.Errorf("internal server error"),
			},
			strategy:    RetryOn50xHTTP,
			shouldRetry: true,
		},
		{
			name:        "RetryOn500HTTP should NOT retry on non-HTTP error",
			err:         fmt.Errorf("network connection error"),
			strategy:    RetryOn50xHTTP,
			shouldRetry: false,
		},
		{
			name: "RetryOn500HTTP should NOT retry on HTTP 404",
			err: insightsclient.HttpError{
				StatusCode: http.StatusNotFound,
				Err:        fmt.Errorf("not found"),
			},
			strategy:    RetryOn50xHTTP,
			shouldRetry: false,
		},

		// RetryOnNon200HTTP strategy tests
		{
			name: "RetryOnNon200HTTP should retry on HTTP 404",
			err: insightsclient.HttpError{
				StatusCode: http.StatusNotFound,
				Err:        fmt.Errorf("not found"),
			},
			strategy:    RetryOnNon200HTTP,
			shouldRetry: true,
		},
		{
			name: "RetryOnNon200HTTP should retry on HTTP 500",
			err: insightsclient.HttpError{
				StatusCode: http.StatusInternalServerError,
				Err:        fmt.Errorf("internal server error"),
			},
			strategy:    RetryOnNon200HTTP,
			shouldRetry: true,
		},
		{
			name:        "RetryOnNon200HTTP should retry on non-HTTP error",
			err:         fmt.Errorf("network error"),
			strategy:    RetryOnNon200HTTP,
			shouldRetry: true,
		},
		{
			name: "RetryOnNon200HTTP should NOT retry on HTTP 200",
			err: insightsclient.HttpError{
				StatusCode: http.StatusOK,
				Err:        nil,
			},
			strategy:    RetryOnNon200HTTP,
			shouldRetry: false,
		},

		// RetryOnAll strategy tests
		{
			name: "RetryOnAll should retry on HTTP 400",
			err: insightsclient.HttpError{
				StatusCode: http.StatusBadRequest,
				Err:        fmt.Errorf("bad request"),
			},
			strategy:    RetryOnAll,
			shouldRetry: true,
		},
		{
			name: "RetryOnAll should retry on HTTP 500",
			err: insightsclient.HttpError{
				StatusCode: http.StatusInternalServerError,
				Err:        fmt.Errorf("internal server error"),
			},
			strategy:    RetryOnAll,
			shouldRetry: true,
		},
		{
			name:        "RetryOnAll should retry on non-HTTP error",
			err:         fmt.Errorf("some random error"),
			strategy:    RetryOnAll,
			shouldRetry: true,
		},
		{
			name: "RetryOnAll should retry even on HTTP 200 with error",
			err: insightsclient.HttpError{
				StatusCode: http.StatusOK,
				Err:        fmt.Errorf("unexpected error despite 200"),
			},
			strategy:    RetryOnAll,
			shouldRetry: true,
		},

		// Unknown strategy tests
		{
			name:        "Unknown strategy should NOT retry",
			err:         fmt.Errorf("some error"),
			strategy:    RetryStrategy(999),
			shouldRetry: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRetry(tt.err, tt.strategy)
			if result != tt.shouldRetry {
				t.Errorf("shouldRetry() = %v, want %v", result, tt.shouldRetry)
			}
		})
	}
}

func Test_RetryWithExpBackOff(t *testing.T) {
	type testCase struct {
		name          string
		backoff       wait.Backoff
		strategy      RetryStrategy
		operation     func() ([]byte, error)
		expectedData  []byte
		expectedError string
	}

	testCases := []testCase{
		{
			name: "successful operation on first try",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() ([]byte, error) {
				return []byte("success"), nil
			},
			expectedData:  []byte("success"),
			expectedError: "",
		},
		{
			name: "successful after 2 retries",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() func() ([]byte, error) {
				attempts := 0
				return func() ([]byte, error) {
					attempts++
					if attempts < 3 {
						return nil, fmt.Errorf("attempt %d failed", attempts)
					}
					return []byte("success after retries"), nil
				}
			}(),
			expectedData:  []byte("success after retries"),
			expectedError: "",
		},
		{
			name: "exhausted retries returns last error",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() ([]byte, error) {
				return nil, fmt.Errorf("persistent failure")
			},
			expectedData:  nil,
			expectedError: "persistent failure",
		},
		{
			name: "single-step backoff returns original error",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    1,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() ([]byte, error) {
				return nil, fmt.Errorf("immediate failure")
			},
			expectedData:  nil,
			expectedError: "immediate failure",
		},
		{
			name: "RetryOn50xHTTP does not retry on HTTP 404",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOn50xHTTP,
			operation: func() ([]byte, error) {
				return nil, insightsclient.HttpError{
					StatusCode: http.StatusNotFound,
					Err:        fmt.Errorf("not found"),
				}
			},
			expectedData:  nil,
			expectedError: "not found",
		},
		{
			name: "RetryOn50xHTTP retries until exhausted on HTTP 500",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOn50xHTTP,
			operation: func() ([]byte, error) {
				return nil, insightsclient.HttpError{
					StatusCode: http.StatusInternalServerError,
					Err:        fmt.Errorf("server error"),
				}
			},
			expectedData:  nil,
			expectedError: "server error",
		},
		{
			name: "RetryOnNon200HTTP succeeds after HTTP 500 then 200",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnNon200HTTP,
			operation: func() func() ([]byte, error) {
				attempts := 0
				return func() ([]byte, error) {
					attempts++
					if attempts < 2 {
						return nil, insightsclient.HttpError{
							StatusCode: http.StatusInternalServerError,
							Err:        fmt.Errorf("temporary server error"),
						}
					}
					return []byte("recovered"), nil
				}
			}(),
			expectedData:  []byte("recovered"),
			expectedError: "",
		},
		{
			name: "RetryOnNon200HTTP does not retry on HTTP 200",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnNon200HTTP,
			operation: func() ([]byte, error) {
				return nil, insightsclient.HttpError{
					StatusCode: http.StatusOK,
					Err:        fmt.Errorf("error despite 200"),
				}
			},
			expectedData:  nil,
			expectedError: "error despite 200",
		},
		{
			name: "counts steps correctly - fails after exact retry count",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    2,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() func() ([]byte, error) {
				attempts := 0
				return func() ([]byte, error) {
					attempts++
					return nil, fmt.Errorf("fail attempt %d", attempts)
				}
			}(),
			expectedData:  nil,
			expectedError: "fail attempt 2",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			data, err := RetryWithExpBackOff(tt.backoff, tt.strategy, tt.operation)

			// Check error
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}

			// Check data
			if tt.expectedData != nil {
				if data == nil {
					t.Errorf("expected data %q, got nil", string(tt.expectedData))
				} else if !bytes.Equal(data, tt.expectedData) {
					t.Errorf("expected data %q, got %q", string(tt.expectedData), string(data))
				}
			}
		})
	}
}
