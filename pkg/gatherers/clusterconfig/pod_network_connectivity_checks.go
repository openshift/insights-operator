package clusterconfig

import (
	"context"
	"encoding/json"
	"time"

	controlplanev1 "github.com/openshift/api/operatorcontrolplane/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherPNCC collects a summary of failed PodNetworkConnectivityChecks.
// Time of the most recently failed check with each reason and message is recorded.
// The checks are requested via a dynamic client and
// then unmarshaled into the appropriate structure.
//
// Resource API: podnetworkconnectivitychecks.controlplane.operator.openshift.io/v1alpha1
// Docs for relevant types: https://pkg.go.dev/github.com/openshift/api/operatorcontrolplane/v1alpha1
//
// * Location in archive: config/podnetworkconnectivitychecks.json
// * Id in config: pod_network_connectivity_checks
// * Since versions:
//   * 4.8+
func (g *Gatherer) GatherPNCC(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherPNCC(ctx, gatherDynamicClient)
}

func getUnsuccessfulChecks(entries []controlplanev1.LogEntry) []controlplanev1.LogEntry {
	var unsuccessful []controlplanev1.LogEntry
	for _, entry := range entries {
		if !entry.Success {
			unsuccessful = append(unsuccessful, entry)
		}
	}
	return unsuccessful
}

func gatherPNCC(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	pnccListUnstruct, err := dynamicClient.Resource(pnccGroupVersionResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	jsonBytes, err := pnccListUnstruct.MarshalJSON()
	if err != nil {
		return nil, []error{err}
	}

	pnccListStruct := controlplanev1.PodNetworkConnectivityCheckList{}
	if err := json.Unmarshal(jsonBytes, &pnccListStruct); err != nil {
		return nil, []error{err}
	}

	unsuccessful := []controlplanev1.LogEntry{}
	for _, pncc := range pnccListStruct.Items {
		unsuccessful = append(unsuccessful, getUnsuccessfulChecks(pncc.Status.Failures)...)
		for _, outage := range pncc.Status.Outages {
			unsuccessful = append(unsuccessful, getUnsuccessfulChecks(outage.StartLogs)...)
			unsuccessful = append(unsuccessful, getUnsuccessfulChecks(outage.EndLogs)...)
		}
	}

	reasons := map[string]map[string]time.Time{}
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
