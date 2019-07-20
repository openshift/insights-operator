package config

import (
	"fmt"
	"time"
)

// Controller defines the standard config for this operator.
type Serialized struct {
	Report      bool   `json:"report"`
	StoragePath string `json:"storagePath"`
	Interval    string `json:"interval"`
	Endpoint    string `json:"endpoint"`
	Impersonate string `json:"impersonate"`
}

func (s *Serialized) ToController() (*Controller, error) {
	cfg := Controller{
		Report:      s.Report,
		StoragePath: s.StoragePath,
		Endpoint:    s.Endpoint,
		Impersonate: s.Impersonate,
	}
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
	if len(cfg.StoragePath) == 0 {
		return nil, fmt.Errorf("storagePath must point to a directory where snapshots can be stored")
	}
	return &cfg, nil
}

// Controller defines the standard config for this operator.
type Controller struct {
	Report      bool
	StoragePath string
	Interval    time.Duration
	Endpoint    string
	Impersonate string

	Username string
	Password string
	Token    string
}
