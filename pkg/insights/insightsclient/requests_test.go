package insightsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
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

func TestClient_RecvGatheringRules(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(testRules))
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
		VersionedAPIPath:     "/apis/config.openshift.io/v1/clusterversions/version",
	}

	gatherConfigClient := configv1client.New(fakeClient)
	insightsClient := New(http.DefaultClient, 0, "", &MockAuthorizer{}, gatherConfigClient)
	gatheringRulesBytes, err := insightsClient.RecvGatheringRules(context.TODO(), endpoint)
	assert.NoError(t, err)
	assert.JSONEq(t, testRules, string(gatheringRulesBytes))
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
