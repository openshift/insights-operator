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
	endpoint := fmt.Sprintf("%s/%s", httpServer.URL, "%s")
	defer httpServer.Close()

	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "342804d0-b57d-46d4-a84e-4a665a6ffe5e",
			Channel:   "stable-4.9",
		},
	}
	cv, err := json.Marshal(clusterVersion)
	assert.NoError(t, err)

	fakeClient := &fakerest.RESTClient{
		Client: fakerest.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(cv)),
			}
			return resp, nil
		}),
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         configv1.GroupVersion,
	}

	configClient := configv1client.New(fakeClient)
	insightsClient := insightsclient.New(http.DefaultClient, 0, "", &MockAuthorizer{}, configClient)
	httpResp, err := insightsClient.GetWithPathParam(context.Background(), endpoint, "test-version", false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)

	data, err := io.ReadAll(httpResp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, testRules, string(data))
}

func TestClient_RecvSCACerts(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(testSCACerts))
		assert.NoError(t, err)
	}))
	endpoint := httpServer.URL
	defer httpServer.Close()

	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "342804d0-b57d-46d4-a84e-4a665a6ffe5e",
			Channel:   "stable-4.9",
		},
	}
	cv, err := json.Marshal(clusterVersion)
	assert.NoError(t, err)

	fakeClient := &fakerest.RESTClient{
		Client: fakerest.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(cv)),
			}
			return resp, nil
		}),
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         configv1.GroupVersion,
	}

	configClient := configv1client.New(fakeClient)
	insightsClient := insightsclient.New(http.DefaultClient, 0, "", &MockAuthorizer{}, configClient)

	architectures := map[string]struct{}{
		"x86_64":  {},
		"aarch64": {},
	}

	expectedResponse := sca.SCAResponse{
		Items: []sca.SCACertData{
			{
				Cert: "testing-cert",
				Key:  "testing-key",
				ID:   "testing-id",
				Metadata: sca.SCACertMetadata{
					Arch: "aarch64",
				},
				OrgID: "testing-org-id",
			},
			{
				Cert: "testing-cert",
				Key:  "testing-key",
				ID:   "testing-id",
				Metadata: sca.SCACertMetadata{
					Arch: "x86_64",
				},
				OrgID: "testing-org-id",
			},
		},
		Kind:  "EntitlementCertificatesList",
		Total: 2,
	}

	certsBytes, err := insightsClient.RecvSCACerts(context.Background(), endpoint, architectures)
	assert.NoError(t, err)

	var certReponse sca.SCAResponse
	assert.NoError(t, json.Unmarshal(certsBytes, &certReponse))
	assert.Equal(t, expectedResponse, certReponse)
}

type MockAuthorizer struct{}

func (ma *MockAuthorizer) Authorize(_ *http.Request) error {
	return nil
}

func (ma *MockAuthorizer) NewSystemOrConfiguredProxy() func(*http.Request) (*url.URL, error) {
	return func(_ *http.Request) (*url.URL, error) {
		return nil, nil
	}
}

func (ma *MockAuthorizer) Token() (string, error) {
	return "", nil
}
