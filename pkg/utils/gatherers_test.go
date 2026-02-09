package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_ShouldBeProcessedNow(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name               string
		lastProcessingTime time.Time
		period             time.Duration
		expected           bool
	}{
		{
			name:               "should process - period elapsed",
			lastProcessingTime: now.Add(-2 * time.Hour),
			period:             1 * time.Hour,
			expected:           true,
		},
		{
			name:               "should not process - period not elapsed",
			lastProcessingTime: now.Add(-30 * time.Minute),
			period:             1 * time.Hour,
			expected:           false,
		},
		{
			name:               "should process - exactly at period boundary",
			lastProcessingTime: now.Add(-1 * time.Hour),
			period:             1 * time.Hour,
			expected:           true,
		},
		{
			name:               "should process - zero period",
			lastProcessingTime: now.Add(-1 * time.Second),
			period:             0,
			expected:           true,
		},
		{
			name:               "should not process - future last processing time",
			lastProcessingTime: now.Add(1 * time.Hour),
			period:             30 * time.Minute,
			expected:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldBeProcessedNow(tt.lastProcessingTime, tt.period)
			assert.Equal(t, tt.expected, result)
		})
	}
}
