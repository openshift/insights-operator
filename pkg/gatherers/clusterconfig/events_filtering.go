package clusterconfig

import (
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
)

// getEventsForInterval() returns events that occoured since last interval
func getEventsForInterval(interval time.Duration, events *v1.EventList) v1.EventList {
	oldestEventTime := time.Now().Add(-interval)
	var filteredEvents v1.EventList
	for i := range events.Items {
		if isEventNew(&events.Items[i], oldestEventTime) {
			filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
		}
	}
	return filteredEvents
}

// isEventNew() returns true if event occoured after given time, otherwise returns false
func isEventNew(event *v1.Event, oldestEventTime time.Time) bool {
	if event.LastTimestamp.After(oldestEventTime) {
		return true
		// if LastTimestamp is zero then try to check the event series
	} else if event.LastTimestamp.IsZero() {
		if event.Series != nil {
			if event.Series.LastObservedTime.After(oldestEventTime) {
				return true
			}
		}
	}
	return false
}

// filterAbnormalEvents returns events that have Type different from "Normal"
func filterAbnormalEvents(events *v1.EventList) v1.EventList {
	var filteredEvents v1.EventList
	for i := range events.Items {
		if isEventAbnormal(&events.Items[i]) {
			filteredEvents.Items = append(filteredEvents.Items, events.Items[i])
		}
	}
	return filteredEvents
}

func isEventAbnormal(event *v1.Event) bool {
	return event.Type != "Normal"
}

// eventListToCompactedEventList() converts EventList into CompactedEventList
func eventListToCompactedEventList(events *v1.EventList) CompactedEventList {
	var compactedEvents CompactedEventList
	for i := range events.Items {
		event := events.Items[i]
		compactedEvent := CompactedEvent{
			Namespace:     event.Namespace,
			LastTimestamp: event.LastTimestamp.Time,
			Reason:        event.Reason,
			Message:       event.Message,
			Type:          event.Type,
		}
		if event.LastTimestamp.Time.IsZero() {
			if event.Series != nil {
				compactedEvent.LastTimestamp = event.Series.LastObservedTime.Time
			}
		}
		compactedEvents.Items = append(compactedEvents.Items, compactedEvent)
	}

	sort.Slice(compactedEvents.Items, func(i, j int) bool {
		return compactedEvents.Items[i].LastTimestamp.Before(compactedEvents.Items[j].LastTimestamp)
	})

	return compactedEvents
}
