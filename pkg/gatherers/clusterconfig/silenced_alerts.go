package clusterconfig

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// GatherSilencedAlerts Collects the alerts that have been silenced.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/silenced_alerts.json
//
// ### Location in archive
// - `config/silenced_alerts.json`
//
// ### Config ID
// `config/silenced_alerts`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherSilencedAlerts(ctx context.Context) ([]record.Record, []error) {
	alertsRESTClient, err := rest.RESTClientFor(g.alertsGatherKubeConfig)
	if err != nil {
		klog.Warningf("Unable to load alerts client, no alerts will be collected: %v", err)
		return nil, nil
	}

	return gatherSilencedAlerts(ctx, alertsRESTClient)
}

func gatherSilencedAlerts(ctx context.Context, alertsClient rest.Interface) ([]record.Record, []error) {
	data, err := alertsClient.Get().AbsPath("api/v2/alerts").
		Param("silenced", "true").
		Param("active", "false").
		Param("inhibited", "false").
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve silenced alerts: %v", err)
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/silenced_alerts.json", Item: marshal.RawByte(data)},
	}

	return records, nil
}
