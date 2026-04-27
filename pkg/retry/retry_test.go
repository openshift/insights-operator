package retry

import (
	"bytes"
	"context"
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

		// Context cancellation tests
		{
			name:        "should NOT retry on context.Canceled error",
			err:         context.Canceled,
			strategy:    RetryOnAll,
			shouldRetry: false,
		},
		{
			name:        "should NOT retry on context.DeadlineExceeded error",
			err:         context.DeadlineExceeded,
			strategy:    RetryOnAll,
			shouldRetry: false,
		},

		// Pointer HttpError tests (real-world scenario)
		{
			name: "RetryOn50xHTTP should retry on *HttpError (pointer) with HTTP 500",
			err: &insightsclient.HttpError{
				StatusCode: http.StatusInternalServerError,
				Err:        fmt.Errorf("internal server error"),
			},
			strategy:    RetryOn50xHTTP,
			shouldRetry: true,
		},
		{
			name: "RetryOn50xHTTP should NOT retry on *HttpError (pointer) with HTTP 404",
			err: &insightsclient.HttpError{
				StatusCode: http.StatusNotFound,
				Err:        fmt.Errorf("not found"),
			},
			strategy:    RetryOn50xHTTP,
			shouldRetry: false,
		},
		{
			name: "RetryOnNon200HTTP should retry on *HttpError (pointer) with HTTP 500",
			err: &insightsclient.HttpError{
				StatusCode: http.StatusInternalServerError,
				Err:        fmt.Errorf("internal server error"),
			},
			strategy:    RetryOnNon200HTTP,
			shouldRetry: true,
		},
		{
			name: "RetryOnNon200HTTP should NOT retry on *HttpError (pointer) with HTTP 200",
			err: &insightsclient.HttpError{
				StatusCode: http.StatusOK,
				Err:        nil,
			},
			strategy:    RetryOnNon200HTTP,
			shouldRetry: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result := shouldRetry(ctx, tt.err, tt.strategy)
			if result != tt.shouldRetry {
				t.Errorf("shouldRetry() = %v, want %v", result, tt.shouldRetry)
			}
		})
	}
}

func Test_RetryWithExpBackOff(t *testing.T) {
	type testCase struct {
		name           string
		backoff        wait.Backoff
		strategy       RetryStrategy
		operation      func() (Result, error)
		expectedResult Result
		expectedError  string
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
			operation: func() (Result, error) {
				return Result{Data: []byte("success")}, nil
			},
			expectedResult: Result{Data: []byte("success")},
			expectedError:  "",
		},
		{
			name: "successful after 2 retries",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() func() (Result, error) {
				attempts := 0
				return func() (Result, error) {
					attempts++
					if attempts < 3 {
						return Result{}, fmt.Errorf("attempt %d failed", attempts)
					}
					return Result{Data: []byte("success after retries")}, nil
				}
			}(),
			expectedResult: Result{Data: []byte("success after retries")},
			expectedError:  "",
		},
		{
			name: "exhausted retries returns last error",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() (Result, error) {
				return Result{}, fmt.Errorf("persistent failure")
			},
			expectedResult: Result{},
			expectedError:  "persistent failure",
		},
		{
			name: "single-step backoff returns original error",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    1,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() (Result, error) {
				return Result{}, fmt.Errorf("immediate failure")
			},
			expectedResult: Result{},
			expectedError:  "immediate failure",
		},
		{
			name: "RetryOn50xHTTP does not retry on HTTP 404",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOn50xHTTP,
			operation: func() (Result, error) {
				return Result{}, insightsclient.HttpError{
					StatusCode: http.StatusNotFound,
					Err:        fmt.Errorf("not found"),
				}
			},
			expectedResult: Result{},
			expectedError:  "not found",
		},
		{
			name: "RetryOn50xHTTP retries until exhausted on HTTP 500",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOn50xHTTP,
			operation: func() (Result, error) {
				return Result{}, insightsclient.HttpError{
					StatusCode: http.StatusInternalServerError,
					Err:        fmt.Errorf("server error"),
				}
			},
			expectedResult: Result{},
			expectedError:  "server error",
		},
		{
			name: "RetryOnNon200HTTP succeeds after HTTP 500 then 200",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnNon200HTTP,
			operation: func() func() (Result, error) {
				attempts := 0
				return func() (Result, error) {
					attempts++
					if attempts < 2 {
						return Result{}, insightsclient.HttpError{
							StatusCode: http.StatusInternalServerError,
							Err:        fmt.Errorf("temporary server error"),
						}
					}
					return Result{Data: []byte("recovered")}, nil
				}
			}(),
			expectedResult: Result{Data: []byte("recovered")},
			expectedError:  "",
		},
		{
			name: "RetryOnNon200HTTP does not retry on HTTP 200",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    3,
				Factor:   2,
			},
			strategy: RetryOnNon200HTTP,
			operation: func() (Result, error) {
				return Result{}, insightsclient.HttpError{
					StatusCode: http.StatusOK,
					Err:        fmt.Errorf("error despite 200"),
				}
			},
			expectedResult: Result{},
			expectedError:  "error despite 200",
		},
		{
			name: "counts steps correctly - fails after exact retry count",
			backoff: wait.Backoff{
				Duration: 1,
				Steps:    2,
				Factor:   2,
			},
			strategy: RetryOnAll,
			operation: func() func() (Result, error) {
				attempts := 0
				return func() (Result, error) {
					attempts++
					return Result{}, fmt.Errorf("fail attempt %d", attempts)
				}
			}(),
			expectedResult: Result{},
			expectedError:  "fail attempt 2",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := RetryWithExpBackOff(ctx, tt.backoff, tt.strategy, tt.operation)

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
			if tt.expectedResult.Data != nil {
				if result.Data == nil {
					t.Errorf("expected data %q, got nil", string(tt.expectedResult.Data))
				} else if !bytes.Equal(result.Data, tt.expectedResult.Data) {
					t.Errorf("expected data %q, got %q", string(tt.expectedResult.Data), string(result.Data))
				}
			}
		})
	}
}
