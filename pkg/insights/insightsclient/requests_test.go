package insightsclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/ocm/sca"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	fakerest "k8s.io/client-go/rest/fake"
)

// MockAuthorizer is a mock implementation of the Authorizer interface for testing
type MockAuthorizer struct {
	token    string
	tokenErr error
	authErr  error
}

func (ma *MockAuthorizer) Authorize(_ *http.Request) error {
	return ma.authErr
}

func (ma *MockAuthorizer) NewSystemOrConfiguredProxy() func(*http.Request) (*url.URL, error) {
	return func(_ *http.Request) (*url.URL, error) {
		return nil, nil
	}
}

func (ma *MockAuthorizer) Token() (string, error) {
	if ma.tokenErr != nil {
		return "", ma.tokenErr
	}
	if ma.token != "" {
		return ma.token, nil
	}
	return "mock-token", nil
}

// Helper function to create fake config client
func createFakeConfigClient(clusterID string) *configv1client.Clientset {
	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID(clusterID),
			Channel:   "stable-4.9",
		},
	}
	cv, _ := json.Marshal(clusterVersion)

	fakeClient := &fakerest.RESTClient{
		Client: fakerest.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(cv)),
			}, nil
		}),
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         configv1.GroupVersion,
	}

	return configv1client.New(fakeClient)
}

const testRules = `{
  "version": "1.0",
  "rules": [
	{
	  "conditions": [
		{
		  "alert": {
			"name": "SamplesImagestreamImportFailing"
		  },
		  "type": "alert_is_firing"
		}
	  ],
	  "gathering_functions": {
		"image_streams_of_namespace": {
		  "namespace": "openshift-cluster-samples-operator"
		},
		"logs_of_namespace": {
		  "namespace": "openshift-cluster-samples-operator",
		  "tail_lines": 100
		}
	  }
	}
  ]
}`

const testSCACerts = `
{
	"items":
		[
			{
				"cert": "testing-cert",
				"key": "testing-key",
				"id": "testing-id",
				"metadata": {
					"arch": "aarch64"
				},
				"organization_id": "testing-org-id",
				"serial": {
					"id": 12345
				}
			},
			{
				"cert": "testing-cert",
				"key": "testing-key",
				"id": "testing-id",
				"metadata": {
					"arch": "x86_64"
				},
				"organization_id": "testing-org-id",
				"serial": {
					"id": 12345
				}
			}
		],
	"kind" : "EntitlementCertificatesList",
	"total" : 2
}
`

func TestClient_RecvGatheringRules(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(testRules))
		assert.NoError(t, err)
	}))
	defer httpServer.Close()

	configClient := createFakeConfigClient("342804d0-b57d-46d4-a84e-4a665a6ffe5e")
	insightsClient := insightsclient.New(http.DefaultClient, 0, "", &MockAuthorizer{}, configClient)

	endpoint := fmt.Sprintf("%s/%%s", httpServer.URL)
	httpResp, err := insightsClient.GetWithPathParam(context.Background(), endpoint, "test-version", false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)

	data, err := io.ReadAll(httpResp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, testRules, string(data))
}

func TestClient_RecvSCACerts(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		responseBody string
		expectError  bool
		errorContains string
	}{
		{
			name:         "success",
			statusCode:   http.StatusOK,
			responseBody: testSCACerts,
			expectError:  false,
		},
		{
			name:          "forbidden error",
			statusCode:    http.StatusForbidden,
			responseBody:  "Forbidden",
			expectError:   true,
			errorContains: "OCM API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				assert.NoError(t, err)
			}))
			defer httpServer.Close()

			configClient := createFakeConfigClient("342804d0-b57d-46d4-a84e-4a665a6ffe5e")
			insightsClient := insightsclient.New(http.DefaultClient, 0, "", &MockAuthorizer{}, configClient)

			architectures := map[string]struct{}{
				"x86_64":  {},
				"aarch64": {},
			}

			certsBytes, err := insightsClient.RecvSCACerts(context.Background(), httpServer.URL, architectures)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, certsBytes)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)

				expectedResponse := sca.Response{
					Items: []sca.CertData{
						{
							Cert:     "testing-cert",
							Key:      "testing-key",
							ID:       "testing-id",
							Metadata: sca.CertMetadata{Arch: "aarch64"},
							OrgID:    "testing-org-id",
						},
						{
							Cert:     "testing-cert",
							Key:      "testing-key",
							ID:       "testing-id",
							Metadata: sca.CertMetadata{Arch: "x86_64"},
							OrgID:    "testing-org-id",
						},
					},
					Kind:  "EntitlementCertificatesList",
					Total: 2,
				}

				var certResponse sca.Response
				assert.NoError(t, json.Unmarshal(certsBytes, &certResponse))
				assert.Equal(t, expectedResponse, certResponse)
			}
		})
	}
}

func TestClient_RecvReport(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectError   bool
		errorContains string
		verifyError   func(t *testing.T, err error)
	}{
		{
			name:         "success",
			statusCode:   http.StatusOK,
			responseBody: `{"report": "test data"}`,
			expectError:  false,
		},
		{
			name:          "unauthorized",
			statusCode:    http.StatusUnauthorized,
			responseBody:  "Unauthorized",
			expectError:   true,
			errorContains: "not enabled for remote support",
		},
		{
			name:         "not found",
			statusCode:   http.StatusNotFound,
			responseBody: "Not found",
			expectError:  true,
			verifyError: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "not found")
				var httpErr insightsclient.HttpError
				assert.ErrorAs(t, err, &httpErr)
				assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
			},
		},
		{
			name:          "bad request",
			statusCode:    http.StatusBadRequest,
			responseBody:  "Bad request",
			expectError:   true,
			errorContains: "bad request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-rh-insights-request-id", "test-request-id")
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				assert.NoError(t, err)
			}))
			defer httpServer.Close()

			configClient := createFakeConfigClient("test-cluster-id")
			insightsClient := insightsclient.New(http.DefaultClient, 0, "test", &MockAuthorizer{}, configClient)

			endpoint := fmt.Sprintf("%s/%%s", httpServer.URL)
			resp, err := insightsClient.RecvReport(context.Background(), endpoint)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, resp)
				if tt.verifyError != nil {
					tt.verifyError(t, err)
				} else {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.Equal(t, http.StatusOK, resp.StatusCode)

				body, err := io.ReadAll(resp.Body)
				assert.NoError(t, err)
				assert.JSONEq(t, tt.responseBody, string(body))
			}
		})
	}
}

func TestClient_RecvClusterTransfer(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectError   bool
		errorContains string
		verifyRequest func(t *testing.T, r *http.Request)
	}{
		{
			name:         "success",
			statusCode:   http.StatusOK,
			responseBody: `{"items": [{"id": "transfer-123"}]}`,
			expectError:  false,
			verifyRequest: func(t *testing.T, r *http.Request) {
				query := r.URL.Query().Get("search")
				assert.Contains(t, query, "cluster_uuid is")
				assert.Contains(t, query, "status is 'accepted'")
				authHeader := r.Header.Get("Authorization")
				assert.Contains(t, authHeader, "AccessToken")
			},
		},
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  "Server error",
			expectError:   true,
			errorContains: "OCM API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.verifyRequest != nil {
					tt.verifyRequest(t, r)
				}
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				assert.NoError(t, err)
			}))
			defer httpServer.Close()

			configClient := createFakeConfigClient("test-cluster-456")
			insightsClient := insightsclient.New(http.DefaultClient, 0, "test", &MockAuthorizer{token: "test-token"}, configClient)

			data, err := insightsClient.RecvClusterTransfer(httpServer.URL)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, data)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, tt.responseBody, string(data))
			}
		})
	}
}

func TestClient_GetWithPathParam(t *testing.T) {
	tests := []struct {
		name             string
		includeClusterID bool
		param            string
		verifyPath       func(t *testing.T, path string)
	}{
		{
			name:             "with cluster ID",
			includeClusterID: true,
			param:            "test-param",
			verifyPath: func(t *testing.T, path string) {
				assert.Contains(t, path, "test-cluster-id")
				assert.Contains(t, path, "test-param")
			},
		},
		{
			name:             "without cluster ID",
			includeClusterID: false,
			param:            "my-param",
			verifyPath: func(t *testing.T, path string) {
				assert.Contains(t, path, "my-param")
				assert.NotContains(t, path, "test-cluster-id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.verifyPath(t, r.URL.Path)
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(`{"data": "test"}`))
				assert.NoError(t, err)
			}))
			defer httpServer.Close()

			configClient := createFakeConfigClient("test-cluster-id")
			insightsClient := insightsclient.New(http.DefaultClient, 0, "test", &MockAuthorizer{}, configClient)

			var endpoint string
			if tt.includeClusterID {
				endpoint = fmt.Sprintf("%s/%%s/%%s", httpServer.URL)
			} else {
				endpoint = fmt.Sprintf("%s/%%s", httpServer.URL)
			}

			resp, err := insightsClient.GetWithPathParam(context.Background(), endpoint, tt.param, tt.includeClusterID)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestClient_SendAndGetID(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		requestID       string
		responseBody    string
		expectError     bool
		errorContains   string
		expectedStatus  int
		verifyRequestID bool
	}{
		{
			name:            "success",
			statusCode:      http.StatusOK,
			requestID:       "test-request-123",
			responseBody:    "",
			expectError:     false,
			expectedStatus:  http.StatusOK,
			verifyRequestID: true,
		},
		{
			name:           "unauthorized",
			statusCode:     http.StatusUnauthorized,
			responseBody:   "Unauthorized",
			expectError:    true,
			errorContains:  "not enabled for remote support",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "forbidden",
			statusCode:     http.StatusForbidden,
			responseBody:   "Forbidden",
			expectError:    true,
			errorContains:  "not enabled for remote support",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "bad request",
			statusCode:     http.StatusBadRequest,
			responseBody:   "Invalid data",
			expectError:    true,
			errorContains:  "bad request",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "Internal error",
			expectError:    true,
			errorContains:  "unexpected error code",
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.requestID != "" {
					w.Header().Set("x-rh-insights-request-id", tt.requestID)
				}
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				assert.NoError(t, err)
			}))
			defer httpServer.Close()

			configClient := createFakeConfigClient("test-cluster-id")
			insightsClient := insightsclient.New(http.DefaultClient, 0, "test", &MockAuthorizer{}, configClient)

			source := insightsclient.Source{
				ID:       "test-source",
				Type:     "application/vnd.redhat.openshift.periodic+tar",
				Contents: io.NopCloser(bytes.NewReader([]byte("test data"))),
			}

			requestID, statusCode, err := insightsClient.SendAndGetID(context.Background(), httpServer.URL, source)

			assert.Equal(t, tt.expectedStatus, statusCode)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}

			if tt.verifyRequestID {
				assert.Equal(t, tt.requestID, requestID)
			}
		})
	}
}
