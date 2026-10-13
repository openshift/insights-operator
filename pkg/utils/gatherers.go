package utils

import "time"

// ShouldBeProcessedNow useful function for gatherers with custom period
func ShouldBeProcessedNow(lastProcessingTime time.Time, period time.Duration) bool {
	timeToProcess := lastProcessingTime.Add(period)
	return time.Now().Equal(timeToProcess) || time.Now().After(timeToProcess)
}
