package config

import (
	"fmt"
	"time"
)

// Serialized defines the standard config for this operator.
type Serialized struct {
	Report      bool   `json:"report"`
	StoragePath string `json:"storagePath"`
	Interval    string `json:"interval"`
	Endpoint    string `json:"endpoint"`
	PullReport  struct {
		Endpoint     string `json:"endpoint"`
		Delay        string `json:"delay"`
		Timeout      string `json:"timeout"`
		MinRetryTime string `json:"min_retry"`
	} `json:"pull_report"`
	Impersonate                  string   `json:"impersonate"`
	Gather                       []string `json:"gather"`
	DisabledGlobalAnonymizations []string `json:"disabledGlobalAnonymizations"`
}

func (s *Serialized) ToController(cfg *Controller) (*Controller, error) {
	if cfg == nil {
		cfg = &Controller{}
	}

	cfg.Report = s.Report
	cfg.StoragePath = s.StoragePath
	cfg.Endpoint = s.Endpoint
	cfg.Impersonate = s.Impersonate
	cfg.Gather = s.Gather

	s.fillAnonymizationConfig(cfg)

	if len(s.Interval) > 0 {
		d, err := time.ParseDuration(s.Interval)
		if err != nil {
			return nil, fmt.Errorf("interval must be a valid duration: %v", err)
		}
		cfg.Interval = d
	}

	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("interval must be a non-negative duration")
	}

	if len(s.PullReport.Endpoint) > 0 {
		cfg.ReportEndpoint = s.PullReport.Endpoint
	}

	if len(s.PullReport.Delay) > 0 {
		d, err := time.ParseDuration(s.PullReport.Delay)
		if err != nil {
			return nil, fmt.Errorf("delay must be a valid duration: %v", err)
		}
		cfg.ReportPullingDelay = d
	}

	if cfg.ReportPullingDelay <= 0 {
		return nil, fmt.Errorf("delay must be a non-negative duration")
	}

	if len(s.PullReport.MinRetryTime) > 0 {
		d, err := time.ParseDuration(s.PullReport.MinRetryTime)
		if err != nil {
			return nil, fmt.Errorf("min_retry must be a valid duration: %v", err)
		}
		cfg.ReportMinRetryTime = d
	}

	if cfg.ReportMinRetryTime <= 0 {
		return nil, fmt.Errorf("min_retry must be a non-negative duration")
	}

	if len(s.PullReport.Timeout) > 0 {
		d, err := time.ParseDuration(s.PullReport.Timeout)
		if err != nil {
			return nil, fmt.Errorf("timeout must be a valid duration: %v", err)
		}
		cfg.ReportPullingTimeout = d
	}

	if cfg.ReportPullingTimeout <= 0 {
		return nil, fmt.Errorf("timeout must be a non-negative duration")
	}

	if len(cfg.StoragePath) == 0 {
		return nil, fmt.Errorf("storagePath must point to a directory where snapshots can be stored")
	}

	return cfg, nil
}

// Controller defines the standard config for this operator.
type Controller struct {
	Report               bool
	StoragePath          string
	Interval             time.Duration
	Endpoint             string
	ReportEndpoint       string
	ReportPullingDelay   time.Duration
	ReportMinRetryTime   time.Duration
	ReportPullingTimeout time.Duration
	Impersonate          string
	Gather               []string
	// DisabledGlobalAnonymizations specifies which of global anonymizations to disable.
	// By default, we anonymize everything.
	// To see the detailed info about what we anonymize, go to the docs of package anonymization.
	DisabledGlobalAnonymizations struct {
		DisableClusterBaseDomainAnonymization bool
	}

	Username string
	Password string
	Token    string

	HTTPConfig HTTPConfig
}

// HTTPConfig configures http proxy and exception settings if they come from config
type HTTPConfig struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}
