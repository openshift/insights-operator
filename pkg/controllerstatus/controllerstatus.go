package controllerstatus

import (
	"sync"
	"time"

	"k8s.io/klog/v2"
)

type Interface interface {
	CurrentStatus() (summary Summary, ready bool)
}

type Operation string

const (
	// DownloadingReport specific flag for Smart Proxy report downloading process.
	DownloadingReport Operation = "DownloadingReport"
	// Uploading specific flag for summary related to uploading process.
	Uploading Operation = "Uploading"
	// GatheringReport specific for gathering the report from the cluster
	GatheringReport Operation = "GatheringReport"
	// PullingSCACerts is specific operation for pulling the SCA certs data from the OCM API
	PullingSCACerts Operation = "PullingSCACerts"
)

// Summary represents the status summary of an Operation
type Summary struct {
	Operation          Operation
	Healthy            bool
	Reason             string
	Message            string
	LastTransitionTime time.Time
	Count              int
}

// Simple represents the status of a given part of the operator
type Simple struct {
	Name string

	lock    sync.Mutex
	summary Summary
}

// UpdateStatus updates the status, keeps track how long a status have been in effect
func (s *Simple) UpdateStatus(summary Summary) { //nolint: gocritic
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

// CurrentStatus retrives the status summary in a thread-safe way
func (s *Simple) CurrentStatus() (Summary, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.summary.Count == 0 {
		return Summary{}, false
	}
	return s.summary, true
}
