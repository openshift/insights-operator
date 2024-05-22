package controllerstatus

import (
	"sync"
	"time"

	"k8s.io/klog/v2"
)

type StatusController interface {
	CurrentStatus() (summary Summary, ready bool)
	UpdateStatus(summary Summary)
	Name() string
}

type Operation struct {
	Name           OperationName
	HTTPStatusCode int
}

type OperationName string

var (
	// DownloadingReport specific flag for Smart Proxy report downloading process.
	DownloadingReport = Operation{Name: "DownloadingReport"}
	// Uploading specific flag for summary related to uploading process.
	Uploading = Operation{Name: "Uploading"}
	// GatheringReport specific for gathering the report from the cluster
	GatheringReport = Operation{Name: "GatheringReport"}
	// PullingSCACerts is specific operation for pulling the SCA certs data from the OCM API
	PullingSCACerts = Operation{Name: "PullingSCACerts"}
	// PullingClusterTransfer is an operator for pulling ClusterTransfer object from the OCM API endpoint
	PullingClusterTransfer = Operation{Name: "PullingClusterTransfer"}

	ReadingRemoteConfiguration = Operation{Name: "ReadingRemoteConfiguration"}
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
	name string

	lock    sync.Mutex
	summary Summary
}

func New(name string) StatusController {
	return &Simple{
		name: name,
	}
}

// UpdateStatus updates the status, keeps track how long a status have been in effect
func (s *Simple) UpdateStatus(summary Summary) { //nolint: gocritic
	s.lock.Lock()
	defer s.lock.Unlock()

	if summary.LastTransitionTime.IsZero() {
		s.summary.LastTransitionTime = time.Now()
	}

	// this is an ugly hack for tech preview with gathering jobs. The reason is that we don't want to count
	// the attempts in this case, because the attempts (e.g upload) happens in the job
	if summary.Count > 0 {
		s.summary = summary
		return
	}

	if s.summary.Healthy != summary.Healthy {
		klog.Infof("name=%s healthy=%t reason=%s message=%s", s.name, summary.Healthy, summary.Reason, summary.Message)
		s.summary = summary
		s.summary.Count = 1
		s.summary.LastTransitionTime = summary.LastTransitionTime
		return
	}

	s.summary.Count++
	if s.summary.Message != summary.Message || s.summary.Reason != summary.Reason {
		klog.Infof("name=%s healthy=%t reason=%s message=%s", s.name, summary.Healthy, summary.Reason, summary.Message)
		s.summary.Reason = summary.Reason
		s.summary.Message = summary.Message
		s.summary.Operation = summary.Operation
		s.summary.LastTransitionTime = summary.LastTransitionTime
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

func (s *Simple) Name() string {
	return s.name
}
