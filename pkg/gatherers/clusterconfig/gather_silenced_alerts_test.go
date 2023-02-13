package clusterconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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

	baseURL, _ := url.Parse(ts.URL)
	client, err := rest.NewRESTClient(baseURL, "", rest.ClientContentConfig{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create a client: %v", err)
	}

	return client
}

func TestGatherSilencedAlerts(t *testing.T) {
	tests := []struct {
		name             string
		mockAlertsClient *mockAlertsClient
		wantRecords      []record.Record
		wantErrsCount    int
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
			wantErrsCount: 0,
		},
		{
			name:             "Get silenced alerts with error",
			mockAlertsClient: &mockAlertsClient{data: nil},
			wantRecords:      nil,
			wantErrsCount:    1,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, errs := gatherSilencedAlerts(ctx, tt.mockAlertsClient.RestClient(t))
			assert.Len(t, errs, tt.wantErrsCount)
			assert.Equal(t, tt.wantRecords, records)
		})
	}
}
