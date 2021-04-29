package config

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
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
	Impersonate             string   `json:"impersonate"`
	Gather                  []string `json:"gather"`
	EnableGlobalObfuscation bool     `json:"enableGlobalObfuscation"`
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
	// list of gathering functions to enable
	// if there's a string "ALL", we enable everything
	// otherwise, each string should consist of 2 parts:
	// gatherer name and function name split by slash
	// Example: []{
	//   "clusterconfig/container_images",
	//   "clusterconfig/nodes",
	//   "clusterconfig/authentication",
	// }
	Gather []string
	// EnableGlobalObfuscation enables obfuscation of domain names and IP addresses
	// To see the detailed info about how anonymization works, go to the docs of package anonymization.
	EnableGlobalObfuscation bool

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

type Converter func(s *Serialized, cfg *Controller) (*Controller, error)

// Returns the important fields of the config in a string form
func (c *Controller) ToString() string {
	return fmt.Sprintf("enabled=%t "+
		"endpoint=%s "+
		"interval=%s "+
		"username=%t "+
		"token=%t "+
		"reportEndpoint=%s "+
		"initialPollingDelay=%s "+
		"minRetryTime=%s "+
		"pollingTimeout=%s",
		c.Report,
		c.Endpoint,
		c.Interval,
		len(c.Username) > 0,
		len(c.Token) > 0,
		c.ReportEndpoint,
		c.ReportPullingDelay,
		c.ReportMinRetryTime,
		c.ReportPullingTimeout)
}

// Creates/updates a config Controller according to the Serialized config.
// Makes sure that the config is correct.
func ToController(s *Serialized, cfg *Controller) (*Controller, error) { // nolint: gocyclo
	if cfg == nil {
		cfg = &Controller{}
	}

	cfg.Report = s.Report
	cfg.StoragePath = s.StoragePath
	cfg.Endpoint = s.Endpoint
	cfg.Impersonate = s.Impersonate
	cfg.Gather = s.Gather
	cfg.EnableGlobalObfuscation = s.EnableGlobalObfuscation

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

// Creates/updates a config Controller according to the Serialized config.
// Makes sure that the config is correct, but only checks fields necessary for disconnected operation.
func ToDisconnectedController(s *Serialized, cfg *Controller) (*Controller, error) {
	if cfg == nil {
		cfg = &Controller{}
	}
	cfg.Report = s.Report
	cfg.StoragePath = s.StoragePath
	cfg.Impersonate = s.Impersonate
	cfg.Gather = s.Gather
	cfg.EnableGlobalObfuscation = s.EnableGlobalObfuscation

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
	return cfg, nil
}

// LoadConfig unmarshalls config from obj and loads it to this Controller struct
func LoadConfig(controller Controller, obj map[string]interface{}, converter Converter) (Controller, error) { //nolint: gocritic
	var cfg Serialized
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &cfg); err != nil {
		return controller, fmt.Errorf("unable to load config: %v", err)
	}

	loadedController, err := converter(&cfg, &controller)
	if err != nil {
		return controller, err
	}
	data, _ := json.Marshal(cfg)
	klog.V(2).Infof("Current config: %s", string(data))
	return *loadedController, nil
}
