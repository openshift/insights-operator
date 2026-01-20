package insightsclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/version"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name          string
		client        *http.Client
		maxBytes      int64
		expectedBytes int64
		expectClient  bool
	}{
		{
			name:          "with nil client",
			client:        nil,
			maxBytes:      1000,
			expectedBytes: 1000,
			expectClient:  true,
		},
		{
			name:          "with zero maxBytes",
			client:        &http.Client{},
			maxBytes:      0,
			expectedBytes: 10 * 1024 * 1024,
			expectClient:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := New(tt.client, tt.maxBytes, "test", nil, nil)
			if tt.expectClient {
				assert.NotNil(t, result.client)
			}
			assert.Equal(t, tt.expectedBytes, result.maxBytes)
		})
	}
}

func TestHttpError(t *testing.T) {
	httpErr := HttpError{Err: assert.AnError, StatusCode: 500}

	assert.True(t, IsHttpError(httpErr))
	assert.False(t, IsHttpError(assert.AnError))
	assert.Contains(t, httpErr.Error(), "assert.AnError")
}

func TestNewHTTPErrorFromResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	resp := &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(bytes.NewReader([]byte("Not Found"))),
		Request:    req,
	}

	httpErr := newHTTPErrorFromResponse(resp)

	assert.NotNil(t, httpErr)
	assert.Equal(t, 404, httpErr.StatusCode)
	assert.Contains(t, httpErr.Error(), "404")
}

func TestResponseBody(t *testing.T) {
	tests := []struct {
		name           string
		response       *http.Response
		expectedResult string
		expectedLen    int
		checkLen       bool
	}{
		{
			name:           "nil response",
			response:       nil,
			expectedResult: "",
			checkLen:       false,
		},
		{
			name: "truncates long body",
			response: &http.Response{
				Body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), 2000))),
			},
			expectedLen: 1024,
			checkLen:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := responseBody(tt.response)
			if tt.checkLen {
				assert.Equal(t, tt.expectedLen, len(result))
			} else {
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestUserAgent(t *testing.T) {
	versionInfo := version.Info{GitVersion: "v1.25.0", GitCommit: "abc123"}
	cv := &configv1.ClusterVersion{
		Spec: configv1.ClusterVersionSpec{ClusterID: "test-cluster"},
	}

	result := userAgent("4.12.0", versionInfo, cv)

	assert.Contains(t, result, "insights-operator/4.12.0-abc123")
	assert.Contains(t, result, "cluster/test-cluster")
}

func TestOcmErrorMessage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://api.openshift.com/api/v1/certs", nil)
	resp := &http.Response{
		StatusCode: 403,
		Body:       io.NopCloser(bytes.NewReader([]byte("Forbidden"))),
		Request:    req,
	}

	err := ocmErrorMessage(resp)

	httpErr, ok := err.(HttpError)
	assert.True(t, ok)
	assert.Equal(t, 403, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "OCM API")
}
