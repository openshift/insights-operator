package clusterconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

type mockAlertsClient struct {
	data []byte
}

func (c *mockAlertsClient) RestClient(t *testing.T) *rest.RESTClient {
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

func TestGatherSilencedAlerts(t *testing.T) {
	tests := []struct {
		name             string
		mockAlertsClient *mockAlertsClient
		wantRecords      []record.Record
		wantErrs         []error
	}{
		{
			name: "Get silenced alerts successfully",
			mockAlertsClient: &mockAlertsClient{
				data: []byte(`[{"status": {"state": "suppressed"}}]`),
			},
			wantRecords: []record.Record{
				{
					Name: "config/silenced_alerts.json",
					Item: marshal.RawByte(`[{"status": {"state": "suppressed"}}]`),
				},
			},
			wantErrs: nil,
		},
		{
			name:             "Get silenced alerts with error",
			mockAlertsClient: &mockAlertsClient{data: nil},
			wantRecords:      nil,
			wantErrs: []error{
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

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, errs := gatherSilencedAlerts(ctx, tt.mockAlertsClient.RestClient(t))
			assert.Equal(t, tt.wantErrs, errs)
			assert.Equal(t, tt.wantRecords, records)
		})
	}
}
