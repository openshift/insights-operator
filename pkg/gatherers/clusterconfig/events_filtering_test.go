package clusterconfig

import (
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_filterEvents_LastTimeStamp(t *testing.T) {
	oldEvent := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "oldEvent"},
		LastTimestamp: metav1.Time{},
		Type:          "Normal",
	}
	newEvent := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "newEvent"},
		LastTimestamp: metav1.Now(),
		Type:          "Normal",
	}

	events := v1.EventList{}
	events.Items = append(events.Items, newEvent, oldEvent)

	filteredEvents := filterEvents(time.Duration(1*time.Minute), &events, "")

	if !reflect.DeepEqual(filteredEvents.Items[0], newEvent) {
		t.Errorf("filterEvents() = %v, want %v", filteredEvents.Items[0], newEvent)
	}
}

func Test_filterEvents_Warning(t *testing.T) {
	normalEvent := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "normalEvent"},
		LastTimestamp: metav1.Now(),
		Type:          "Normal",
	}
	warningEvent := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "warningEvent"},
		LastTimestamp: metav1.Now(),
		Type:          "Warning",
	}

	events := v1.EventList{}
	events.Items = append(events.Items, normalEvent, warningEvent)

	filteredEvents := filterEvents(time.Duration(1*time.Minute), &events, "Warning")

	if !reflect.DeepEqual(filteredEvents.Items[0], warningEvent) {
		t.Errorf("filterEvents() = %v, want %v", filteredEvents.Items[0], warningEvent)
	}
}

func Test_eventListToCompactedEventList(t *testing.T) {
	timeNow := time.Now()
	event := v1.Event{
		ObjectMeta:    metav1.ObjectMeta{Name: "event", Namespace: "test namespace"},
		LastTimestamp: metav1.NewTime(timeNow),
		Type:          "Normal",
		Reason:        "test reason",
		Message:       "test message",
	}
	compactedEvent := CompactedEvent{
		Namespace:     "test namespace",
		LastTimestamp: timeNow,
		Reason:        "test reason",
		Message:       "test message",
		Type:          "Normal",
	}
	compactedEventList := eventListToCompactedEventList(v1.EventList{Items: []v1.Event{event}})

	if !reflect.DeepEqual(compactedEvent, compactedEventList.Items[0]) {
		t.Errorf("eventListToCompactedEventList() = %v, want %v", compactedEventList.Items[0], compactedEvent)
	}
}
