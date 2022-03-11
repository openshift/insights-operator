package insightsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	httpClient := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(testRules))
		if err != nil {
			assert.NoError(t, err)
		}
	}))
	defer httpClient.Close()

	endpoint := httpClient.URL

	client := New(http.DefaultClient, 0, "", &MockAuthorizer{}, nil)
	client.clusterVersion = &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "342804d0-b57d-46d4-a84e-4a665a6ffe5e",
			Channel:   "stable-4.9",
		},
	}
	gatheringRulesBytes, err := client.RecvGatheringRules(context.TODO(), endpoint)
	if err != nil {
		assert.NoError(t, err)
	}

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
