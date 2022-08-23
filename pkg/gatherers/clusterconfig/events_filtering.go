package clusterconfig

import (
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
)

//TODO: Simplify if logic
func filterEvents(interval time.Duration, events *v1.EventList, Type string) v1.EventList {
	oldestEventTime := time.Now().Add(-interval)
	var filteredEvents = v1.EventList{}
	switch Type {
	case "Warning":
		for i := range events.Items {
			if events.Items[i].Type != "Normal" {
				// if LastTimestamp is zero then try to check the event series
				if events.Items[i].LastTimestamp.IsZero() {
					if events.Items[i].Series != nil {
						if events.Items[i].Series.LastObservedTime.Time.After(oldestEventTime) {
							filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
						}
					}
				} else {
					if events.Items[i].LastTimestamp.Time.After(oldestEventTime) {
						filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
					}
				}
			}
		}
	default:
		for i := range events.Items {
			// if LastTimestamp is zero then try to check the event series
			if events.Items[i].LastTimestamp.IsZero() {
				if events.Items[i].Series != nil {
					if events.Items[i].Series.LastObservedTime.Time.After(oldestEventTime) {
						filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
					}
				}
			} else {
				if events.Items[i].LastTimestamp.Time.After(oldestEventTime) {
					filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
				}
			}
		}
	}

	return filteredEvents
}

func eventListToCompactedEventList(events v1.EventList) CompactedEventList {
	compactedEvents := CompactedEventList{Items: make([]CompactedEvent, len(events.Items))}
	for i := 0; i < len(events.Items); i++ {
		compactedEvents.Items[i] = CompactedEvent{
			Namespace:     events.Items[i].Namespace,
			LastTimestamp: events.Items[i].LastTimestamp.Time,
			Reason:        events.Items[i].Reason,
			Message:       events.Items[i].Message,
			Type:          events.Items[i].Type,
		}
		if events.Items[i].LastTimestamp.Time.IsZero() {
			compactedEvents.Items[i].LastTimestamp = events.Items[i].Series.LastObservedTime.Time
		}
	}
	sort.Slice(compactedEvents.Items, func(i, j int) bool {
		return compactedEvents.Items[i].LastTimestamp.Before(compactedEvents.Items[j].LastTimestamp)
	})

	return compactedEvents
}
