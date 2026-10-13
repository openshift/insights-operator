package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherActiveAlerts(t *testing.T) {
	responseData := `
[
  {
    "annotations": {
      "description": "This is just an sample alert description",
      "summary": "An alert sample summary."
    },
    "endsAt": "2023-02-07T15:19:22.206Z",
    "fingerprint": "6934731368443c07",
    "receivers": [
      {
        "name": "Watchdog"
      }
    ],
    "startsAt": "2023-02-07T13:16:22.206Z",
    "status": {
      "inhibitedBy": [],
      "silencedBy": [],
      "state": "active"
    },
    "updatedAt": "2023-02-07T15:15:22.207Z",
    "generatorURL": "https://console-openshift-console.apps.cluster.test/monitoring/graph?g0.expr=vector%281%29&g0.tab=1",
    "labels": {
      "alertname": "Watchdog",
      "namespace": "openshift-monitoring",
      "openshift_io_alert_source": "platform",
      "prometheus": "openshift-monitoring/k8s",
      "severity": "none"
    }
  }
]`

	tests := []struct {
		name             string
		mockAlertsClient *mockAlertsClient
		wantRecords      []record.Record
		wantErrsCount    int
	}{
		{
			name: "Get active alerts successfully",
			mockAlertsClient: &mockAlertsClient{
				data: []byte(responseData),
			},
			wantRecords: []record.Record{
				{
					Name: "config/alerts",
					Item: record.JSONMarshaller{Object: []alert{
						{
							Labels: map[string]string{
								"alertname":                 "Watchdog",
								"namespace":                 "openshift-monitoring",
								"openshift_io_alert_source": "platform",
								"prometheus":                "openshift-monitoring/k8s",
								"severity":                  "none",
							},
							Annotations: map[string]string{
								"description": "This is just an sample alert description",
								"summary":     "An alert sample summary.",
							},
							EndsAt:    "2023-02-07T15:19:22.206Z",
							StartsAt:  "2023-02-07T13:16:22.206Z",
							UpdatedAt: "2023-02-07T15:15:22.207Z",
							Status: map[string]interface{}{
								"inhibitedBy": []interface{}{},
								"silencedBy":  []interface{}{},
								"state":       "active",
							},
						},
					}},
				},
			},
			wantErrsCount: 0,
		},
		{
			name:             "Get active alerts with error",
			mockAlertsClient: &mockAlertsClient{data: nil},
			wantRecords:      nil,
			wantErrsCount:    1,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, errs := gatherActiveAlerts(ctx, tt.mockAlertsClient.RestClient(t))
			assert.Len(t, errs, tt.wantErrsCount)
			assert.Equal(t, tt.wantRecords, records)
		})
	}
}
