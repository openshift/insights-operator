package controllerstatus

import (
	"sync"
	"time"

	"k8s.io/klog"
)

type Interface interface {
	CurrentStatus() (summary Summary, ready bool)
}

type Operation string

const (
	// Specific flag for summary related to uploading process.
	Uploading Operation = "Uploading"

	// Specific flag for summary related to gathering process.
	GatheringReport Operation = "GatheringReport"
)

type Summary struct {
	Operation          Operation
	Healthy            bool
	Reason             string
	Message            string
	LastTransitionTime time.Time
	Count              int
}

type Simple struct {
	Name string

	lock    sync.Mutex
	summary Summary
}

func (s *Simple) UpdateStatus(summary Summary) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.summary.Healthy != summary.Healthy {
		klog.V(2).Infof("name=%s healthy=%t reason=%s message=%s", s.Name, summary.Healthy, summary.Reason, summary.Message)
		if summary.LastTransitionTime.IsZero() {
			summary.LastTransitionTime = time.Now()
		}

		s.summary = summary
		s.summary.Count = 1
		return
	}

	s.summary.Count++
	if summary.Healthy {
		return
	}
	if s.summary.Message != summary.Message || s.summary.Reason != summary.Reason {
		klog.V(2).Infof("name=%s healthy=%t reason=%s message=%s", s.Name, summary.Healthy, summary.Reason, summary.Message)
		s.summary.Reason = summary.Reason
		s.summary.Message = summary.Message
		s.summary.Operation = summary.Operation
		return
	}
}

func (s *Simple) CurrentStatus() (Summary, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.summary.Count == 0 {
		return Summary{}, false
	}
	return s.summary, true
}
