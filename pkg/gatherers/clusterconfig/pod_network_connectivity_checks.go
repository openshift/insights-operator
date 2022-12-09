package clusterconfig

import (
	"context"
	"time"

	controlplanev1 "github.com/openshift/api/operatorcontrolplane/v1alpha1"
	ocpV1AlphaCli "github.com/openshift/client-go/operatorcontrolplane/clientset/versioned/typed/operatorcontrolplane/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherPNCC Collects a summary of failed PodNetworkConnectivityChecks from last 24 hours.
//
// ### API Reference
// - podnetworkconnectivitychecks.controlplane.operator.openshift.io/v1alpha1
// - https://pkg.go.dev/github.com/openshift/api/operatorcontrolplane/v1alpha1
//
// ### Sample data
// - docs/insights-archive-sample/config/podnetworkconnectivitychecks.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.8.2  | config/podnetworkconnectivitychecks.json 					|
//
// ### Config ID
// `clusterconfig/pod_network_connectivity_checks`
//
// ### Released version
// - 4.8.2
//
// ### Backported versions
// None
//
// ### Notes
// Time of the most recently failed check with each reason and message is recorded.
func (g *Gatherer) GatherPNCC(ctx context.Context) ([]record.Record, []error) {
	gatherClient, err := ocpV1AlphaCli.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPNCC(ctx, gatherClient)
}

func getUnsuccessfulChecks(entries []controlplanev1.LogEntry) []controlplanev1.LogEntry {
	var unsuccessful []controlplanev1.LogEntry
	t := &metav1.Time{
		Time: time.Now().Add(-24 * time.Hour),
	}
	for _, entry := range entries {
		if entry.Start.Before(t) {
			continue
		}
		if !entry.Success {
			unsuccessful = append(unsuccessful, entry)
		}
	}
	return unsuccessful
}

func gatherPNCC(ctx context.Context, cli ocpV1AlphaCli.ControlplaneV1alpha1Interface) ([]record.Record, []error) {
	pnccList, err := cli.PodNetworkConnectivityChecks("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	unsuccessful := []controlplanev1.LogEntry{}
	for idx := range pnccList.Items {
		pncc := pnccList.Items[idx]
		unsuccessful = append(unsuccessful, getUnsuccessfulChecks(pncc.Status.Failures)...)
		for _, outage := range pncc.Status.Outages {
			unsuccessful = append(unsuccessful, getUnsuccessfulChecks(outage.StartLogs)...)
			unsuccessful = append(unsuccessful, getUnsuccessfulChecks(outage.EndLogs)...)
		}
	}

	reasons := make(map[string]map[string]time.Time, len(unsuccessful))
	for _, entry := range unsuccessful {
		if _, exists := reasons[entry.Reason]; !exists {
			reasons[entry.Reason] = map[string]time.Time{}
		}
		if oldTime, exists := reasons[entry.Reason][entry.Message]; !exists || entry.Start.After(oldTime) {
			reasons[entry.Reason][entry.Message] = entry.Start.Time
		}
	}

	return []record.Record{{Name: "config/podnetworkconnectivitychecks", Item: record.JSONMarshaller{Object: reasons}}}, nil
}
