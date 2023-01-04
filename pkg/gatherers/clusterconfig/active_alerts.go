package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// we could use e.g https://pkg.go.dev/github.com/prometheus/alertmanager@v0.23.0/api/v2/models#GettableAlert,
// but this allows us to control what attributes we want to include in the alert definition
type alert struct {
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]string      `json:"annotations"`
	EndsAt      string                 `json:"endsAt"`
	StartsAt    string                 `json:"startsAt"`
	UpdatedAt   string                 `json:"updatedAt"`
	Status      map[string]interface{} `json:"status"`
}

// GatherActiveAlerts Collects active alerts from the Alertmanager API V2 in the JSON format. Alert data is also
// still included in the [GatherMostRecentMetrics](#mostrecentmetrics) gatherer.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/alerts.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.12   | config/alerts.json				                        	|
//
// ### Config ID
// `clusterconfig/active_alerts`
//
// ### Released version
// - 4.12
//
// ### Backported versions
// None
//
// ### Notes
// This adds new gatherer for gathering firing/active Prometheus alerts in JSON format as well. The original recent
// metrics gatherer still continues to gather the alerts (not in JSON) as well, but this can be removed in the future,
// and we will keep the data only in JSON.
func (g *Gatherer) GatherActiveAlerts(ctx context.Context) ([]record.Record, []error) {
	alertsRESTClient, err := rest.RESTClientFor(g.alertsGatherKubeConfig)
	if err != nil {
		klog.Warningf("Unable to load alerts client, no alerts will be collected: %v", err)
		return nil, nil
	}

	return gatherActiveAlerts(ctx, alertsRESTClient)
}

func gatherActiveAlerts(ctx context.Context, alertsClient rest.Interface) ([]record.Record, []error) {
	alertsData, err := alertsClient.Get().AbsPath("api/v2/alerts").Param("active", "true").DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve most recent alerts: %v", err)
		return nil, []error{err}
	}

	// the maximal number of recorded alerts
	var limit = 1000

	var alerts []alert
	err = json.Unmarshal(alertsData, &alerts)
	if err != nil {
		klog.Errorf("Unable to unmarshall alerts data: %v", err)
		return nil, []error{err}
	}
	var errs []error
	if len(alerts) > limit {
		originalCount := len(alerts)
		alerts = alerts[:limit]
		errs = append(errs, fmt.Errorf("alert limit %d was exceeded! There were %d alerts", limit, originalCount))
	}
	records := []record.Record{
		{Name: "config/alerts", Item: record.JSONMarshaller{Object: alerts}},
	}

	return records, errs
}
