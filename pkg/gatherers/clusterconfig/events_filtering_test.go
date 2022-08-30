package clusterconfig

import (
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getEventsForInterval(t *testing.T) {
	timeNow := time.Now()
	test := struct {
		events   v1.EventList
		expected v1.EventList
	}{
		events: v1.EventList{
			Items: []v1.Event{
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "oldEvent1"},
					LastTimestamp: metav1.Time{},
				},
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "newEvent1"},
					LastTimestamp: metav1.NewTime(timeNow),
				},
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "oldEvent2"},
					LastTimestamp: metav1.Time{},
				},
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "newEvent2"},
					LastTimestamp: metav1.NewTime(timeNow),
				},
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "newEvent3"},
					LastTimestamp: metav1.NewTime(timeNow),
				},
			},
		},
		expected: v1.EventList{
			Items: []v1.Event{
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "newEvent1"},
					LastTimestamp: metav1.NewTime(timeNow),
				},
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "newEvent2"},
					LastTimestamp: metav1.NewTime(timeNow),
				},
				{
					ObjectMeta:    metav1.ObjectMeta{Name: "newEvent3"},
					LastTimestamp: metav1.NewTime(timeNow),
				},
			},
		},
	}

	filteredEvents := getEventsForInterval(1*time.Minute, &test.events)
	if !reflect.DeepEqual(filteredEvents, test.expected) {
		t.Errorf("filterEvents() = %v, want %v", filteredEvents, test.expected)
	}
}

func Test_filterAbnormalEvents(t *testing.T) {
	test := struct {
		events   v1.EventList
		expected v1.EventList
	}{
		events: v1.EventList{
			Items: []v1.Event{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "normalEvent1"},
					Type:       "Normal",
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "warningEvent1"},
					Type:       "Warning",
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "normalEvent2"},
					Type:       "Normal",
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "warningEvent2"},
					Type:       "Warning",
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "warningEvent3"},
					Type:       "Warning",
				},
			},
		},
		expected: v1.EventList{
			Items: []v1.Event{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "warningEvent1"},
					Type:       "Warning",
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "warningEvent2"},
					Type:       "Warning",
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "warningEvent3"},
					Type:       "Warning",
				},
			},
		},
	}

	filteredEvents := filterAbnormalEvents(&test.events)
	if !reflect.DeepEqual(filteredEvents, test.expected) {
		t.Errorf("filterEvents() = %v, want %v", filteredEvents, test.expected)
	}
}

func Test_isEventNew(t *testing.T) {
	tests := []struct {
		event    v1.Event
		expected bool
	}{
		{
			event: v1.Event{
				ObjectMeta:    metav1.ObjectMeta{Name: "newEvent"},
				LastTimestamp: metav1.Now(),
				Type:          "Normal",
			},
			expected: true,
		},
		{
			event: v1.Event{
				ObjectMeta:    metav1.ObjectMeta{Name: "oldEvent"},
				LastTimestamp: metav1.NewTime(time.Now().Add(-6 * time.Minute)),
				Type:          "Normal",
			},
			expected: false,
		},
	}

	for _, test := range tests {
		if isEventNew(&test.event, time.Now().Add(-5*time.Minute)) != test.expected {
			t.Errorf("isEventNew() = %v, got %v", !test.expected, test.expected)
		}
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
	compactedEventList := eventListToCompactedEventList(&v1.EventList{Items: []v1.Event{event}})

	if !reflect.DeepEqual(compactedEvent, compactedEventList.Items[0]) {
		t.Errorf("eventListToCompactedEventList() = %v, want %v", compactedEventList.Items[0], compactedEvent)
	}
}
