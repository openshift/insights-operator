package clusterconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/utils/marshal"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

type mockMostRecentMetricsClient struct {
	data []byte
}

func (c *mockMostRecentMetricsClient) RestClient(t *testing.T) *rest.RESTClient {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.data == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(c.data)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
	}))

	baseURL, err := url.Parse(ts.URL)
	assert.NoError(t, err, "failed to parse server URL")
	client, err := rest.NewRESTClient(baseURL, "", rest.ClientContentConfig{}, nil, nil)
	assert.NoError(t, err, "failed to create the client")

	return client
}

func Test_gatherMostRecentMetrics(t *testing.T) {
	tests := []struct {
		name          string
		metricsClient *mockMostRecentMetricsClient
		wantRecords   []record.Record
		wantErrors    []error
	}{
		{
			name:          "get recent metrics",
			metricsClient: &mockMostRecentMetricsClient{data: []byte(`test`)},
			wantRecords: []record.Record{
				{
					Name: "config/metrics",
					Item: marshal.RawByte(`test# ALERTS 1/1000
test`),
				},
			},
			wantErrors: nil,
		},
		{
			name:          "fail to get recent metrics",
			metricsClient: &mockMostRecentMetricsClient{data: nil},
			wantRecords:   nil,
			wantErrors: []error{
				&errors.StatusError{ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Message: "the server could not find the requested resource",
					Reason:  metav1.StatusReasonNotFound,
					Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    metav1.CauseTypeUnexpectedServerResponse,
								Message: "",
								Field:   "",
							},
						},
					},
					Code: http.StatusNotFound,
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			records, errs := gatherMostRecentMetrics(ctx, tt.metricsClient.RestClient(t))
			assert.Equal(t, tt.wantRecords, records)
			assert.Equal(t, tt.wantErrors, errs)
		})
	}
}
