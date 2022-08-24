package clusterconfig

import (
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
)

//filterEvents() returns events that occoured since last interval
func filterEvents(interval time.Duration, events *v1.EventList, Type string) v1.EventList {
	oldestEventTime := time.Now().Add(-interval)
	var filteredEvents = v1.EventList{}
	switch Type {
	case "Warning":
		for i := range events.Items {
			if isEventAbnormal(&events.Items[i]) && isEventNew(&events.Items[i], oldestEventTime) {
				filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
			}
		}
	default:
		for i := range events.Items {
			if isEventNew(&events.Items[i], oldestEventTime) {
				filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
			}
		}
	}

	return filteredEvents
}

// if LastTimestamp is zero then try to check the event series
func isEventNew(event *v1.Event, oldestEventTime time.Time) bool {
	if event.LastTimestamp.Time.After(oldestEventTime) {
		return true
	} else if event.LastTimestamp.IsZero() {
		if event.Series != nil {
			if event.Series.LastObservedTime.Time.After(oldestEventTime) {
				return true
			}
		}
	}
	return false
}

func isEventAbnormal(event *v1.Event) bool {
	return event.Type != "Normal"
}

//eventListToCompactedEventList() coverts EventList() into CompactedEventList()
func eventListToCompactedEventList(events *v1.EventList) CompactedEventList {
	compactedEvents := CompactedEventList{Items: make([]CompactedEvent, len(events.Items))}
	for i := range events.Items {
		compactedEvents.Items[i] = CompactedEvent{
			Namespace:     events.Items[i].Namespace,
			LastTimestamp: events.Items[i].LastTimestamp.Time,
			Reason:        events.Items[i].Reason,
			Message:       events.Items[i].Message,
			Type:          events.Items[i].Type,
		}
		if events.Items[i].LastTimestamp.Time.IsZero() {
			if events.Items[i].Series != nil {
				compactedEvents.Items[i].LastTimestamp = events.Items[i].Series.LastObservedTime.Time
			}
		}
	}
	sort.Slice(compactedEvents.Items, func(i, j int) bool {
		return compactedEvents.Items[i].LastTimestamp.Before(compactedEvents.Items[j].LastTimestamp)
	})

	return compactedEvents
}
