package insightsclient

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"

	"github.com/openshift/insights-operator/pkg/config"
)

type mockConfig struct {
	caCert []byte
}

func (m *mockConfig) Config() *config.InsightsConfiguration {
	return &config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			CACert: m.caCert,
		},
	}
}

func generateTestCACert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	assert.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

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
			result := NewInsightsClient(tt.client, tt.maxBytes, "test", nil, nil, nil)
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

func Test_getRootCAs(t *testing.T) {
	validCert := generateTestCACert(t)

	tests := []struct {
		name      string
		config    Config
		wantPool  bool
		wantErr   bool
		errString string
	}{
		{
			name:     "nil config returns nil",
			config:   nil,
			wantPool: false,
		},
		{
			name:     "config with empty CACert returns nil",
			config:   &mockConfig{caCert: []byte{}},
			wantPool: false,
		},
		{
			name:     "config with valid CACert returns pool",
			config:   &mockConfig{caCert: validCert},
			wantPool: true,
		},
		{
			name:      "config with invalid PEM returns error",
			config:    &mockConfig{caCert: []byte("not-a-valid-cert")},
			wantErr:   true,
			errString: "error loading configured CACert: invalid PEM data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := getRootCAs(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
				assert.Nil(t, pool)
			} else {
				assert.NoError(t, err)
				if tt.wantPool {
					assert.NotNil(t, pool)
				} else {
					assert.Nil(t, pool)
				}
			}
		})
	}
}
