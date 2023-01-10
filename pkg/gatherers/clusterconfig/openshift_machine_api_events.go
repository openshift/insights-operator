package clusterconfig

import (
	"context"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GatherOpenshiftMachineAPIEvents Collects warning ("abnormal") events
// from `openshift-machine-api` namespace
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/events/openshift-machine-api.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.12.0 | events/openshift-machine-api.json 	                      	|
//
// ### Config ID
// `clusterconfig/openshift_machine_api_events`
//
// ### Released version
// - 4.12.0
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherOpenshiftMachineAPIEvents(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	records, err := gatherOpenshiftMachineAPIEvents(ctx, gatherKubeClient.CoreV1(), g.interval)
	if err != nil {
		return nil, []error{err}
	}
	return records, nil
}

func gatherOpenshiftMachineAPIEvents(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	interval time.Duration) ([]record.Record, error) {
	events, err := coreClient.Events("openshift-machine-api").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// filter the event list to only recent events with type different than "Normal"
	filteredEvents := getEventsForInterval(interval, events)
	filteredEvents = filterAbnormalEvents(&filteredEvents)

	if len(filteredEvents.Items) == 0 {
		return nil, nil
	}
	compactedEvents := eventListToCompactedEventList(&filteredEvents)

	return []record.Record{{Name: "events/openshift-machine-api", Item: record.JSONMarshaller{Object: &compactedEvents}}}, nil
}
