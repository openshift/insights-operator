package clusterconfig

import (
	"context"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GatherOpenshiftMachineAPIEvents collects warning ("abnormal") events
// from "openshift-machine-api" namespace
//
// * Location of events in archive: events/
// * Id in config: clusterconfig/openshift_machine_api_events
// * Since versions:
// 	 * 4.12+
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
