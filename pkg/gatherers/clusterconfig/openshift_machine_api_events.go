package clusterconfig

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GatherOpenshiftMachineApiEvents collects warning ("abnormal") events
// from "openshift-machine-api" namespace
//
// *Location of events in archive: config/events/
func (g *Gatherer) GatherOpenshiftMachineApiEvents(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	namespace := "openshift-machine-api"
	records, err := gatherOpenshiftMachineApiEvents(ctx, gatherKubeClient.CoreV1(), namespace, g.interval)
	if err != nil {
		return nil, []error{err}
	}
	return records, nil
}

func gatherOpenshiftMachineApiEvents(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	namespace string,
	interval time.Duration) ([]record.Record, error) {
	events, err := coreClient.Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// filter the event list to only recent events with type different than "Normal"
	oldestEventTime := time.Now().Add(-interval)
	var filteredEventIndex []int
	for i := range events.Items {
		if events.Items[i].Type != "Normal" { //TODO see if there is types enum
			if events.Items[i].LastTimestamp.IsZero() {
				if events.Items[i].Series != nil {
					if events.Items[i].Series.LastObservedTime.Time.After(oldestEventTime) {
						filteredEventIndex = append(filteredEventIndex, i)
					}
				}
			} else {
				if events.Items[i].LastTimestamp.Time.After(oldestEventTime) {
					filteredEventIndex = append(filteredEventIndex, i)
				}
			}
		}
	}
	if len(filteredEventIndex) == 0 {
		return nil, nil
	}
	compactedEvents := CompactedEventList{Items: make([]CompactedEvent, len(filteredEventIndex))} //TODO check if this is good output
	for i, index := range filteredEventIndex {
		compactedEvents.Items[i] = CompactedEvent{
			Namespace:     events.Items[index].Namespace,
			LastTimestamp: events.Items[index].LastTimestamp.Time,
			Reason:        events.Items[index].Reason,
			Message:       events.Items[index].Message,
		}
		if events.Items[index].LastTimestamp.Time.IsZero() {
			compactedEvents.Items[i].LastTimestamp = events.Items[index].Series.LastObservedTime.Time
		}
	}
	sort.Slice(compactedEvents.Items, func(i, j int) bool {
		return compactedEvents.Items[i].LastTimestamp.Before(compactedEvents.Items[j].LastTimestamp)
	})

	return []record.Record{{Name: fmt.Sprintf("config/events/%s", namespace), Item: record.JSONMarshaller{Object: &compactedEvents}}}, nil
}
