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
	Report                      bool   `json:"report"`
	StoragePath                 string `json:"storagePath"`
	Interval                    string `json:"interval"`
	Endpoint                    string `json:"endpoint"`
	ConditionalGathererEndpoint string `json:"conditionalGathererEndpoint"`
	PullReport                  struct {
		Endpoint     string `json:"endpoint"`
		Delay        string `json:"delay"`
		Timeout      string `json:"timeout"`
		MinRetryTime string `json:"min_retry"`
	} `json:"pull_report"`
	Impersonate             string `json:"impersonate"`
	EnableGlobalObfuscation bool   `json:"enableGlobalObfuscation"`
	OCM                     struct {
		SCAEndpoint             string `json:"scaEndpoint"`
		SCAInterval             string `json:"scaInterval"`
		SCADisabled             bool   `json:"scaDisabled"`
		ClusterTransferEndpoint string `json:"clusterTransferEndpoint"`
		ClusterTransferInterval string `json:"clusterTransferInterval"`
	}
	DisableInsightsAlerts bool `json:"disableInsightsAlerts"`
}

// Controller defines the standard config for this operator.
type Controller struct {
	Report                      bool
	StoragePath                 string
	Interval                    time.Duration
	Endpoint                    string
	ConditionalGathererEndpoint string
	ReportEndpoint              string
	ReportPullingDelay          time.Duration
	ReportMinRetryTime          time.Duration
	ReportPullingTimeout        time.Duration
	Impersonate                 string
	// EnableGlobalObfuscation enables obfuscation of domain names and IP addresses
	// To see the detailed info about how anonymization works, go to the docs of package anonymization.
	EnableGlobalObfuscation bool

	Username string
	Password string
	Token    string

	HTTPConfig HTTPConfig
	OCMConfig  OCMConfig

	// DisableInsightsAlerts disabled exposing of Insights recommendations as Prometheus info alerts
	DisableInsightsAlerts bool
}

// HTTPConfig configures http proxy and exception settings if they come from config
type HTTPConfig struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

// OCMConfig configures the interval and endpoint for retrieving the data from OCM API
type OCMConfig struct {
	SCAInterval             time.Duration
	SCAEndpoint             string
	SCADisabled             bool
	ClusterTransferEndpoint string
	ClusterTransferInterval time.Duration
}

type Converter func(s *Serialized, cfg *Controller) (*Controller, error)

// ToString returns the important fields of the config in a string form
func (c *Controller) ToString() string {
	return fmt.Sprintf("enabled=%t "+
		"endpoint=%s "+
		"conditional_gatherer_endpoint=%s "+
		"interval=%s "+
		"username=%t "+
		"token=%t "+
		"reportEndpoint=%s "+
		"initialPollingDelay=%s "+
		"minRetryTime=%s "+
		"pollingTimeout=%s",
		c.Report,
		c.Endpoint,
		c.ConditionalGathererEndpoint,
		c.Interval,
		len(c.Username) > 0,
		len(c.Token) > 0,
		c.ReportEndpoint,
		c.ReportPullingDelay,
		c.ReportMinRetryTime,
		c.ReportPullingTimeout)
}

func (c *Controller) MergeWith(cfg *Controller) {
	c.mergeCredentials(cfg)
	c.mergeInterval(cfg)
	c.mergeEndpoint(cfg)
	c.mergeConditionalGathererEndpoint(cfg)
	c.mergeReport(cfg)
	c.mergeOCM(cfg)
	c.mergeHTTP(cfg)
}

func (c *Controller) mergeCredentials(cfg *Controller) {
	c.Username = cfg.Username
	c.Password = cfg.Password
}

func (c *Controller) mergeEndpoint(cfg *Controller) {
	if len(cfg.Endpoint) > 0 {
		c.Endpoint = cfg.Endpoint
	}
}

func (c *Controller) mergeConditionalGathererEndpoint(cfg *Controller) {
	if len(cfg.ConditionalGathererEndpoint) > 0 {
		c.ConditionalGathererEndpoint = cfg.ConditionalGathererEndpoint
	}
}

func (c *Controller) mergeReport(cfg *Controller) {
	if len(cfg.ReportEndpoint) > 0 {
		c.ReportEndpoint = cfg.ReportEndpoint
	}
	if cfg.ReportPullingDelay >= 0 {
		c.ReportPullingDelay = cfg.ReportPullingDelay
	}
	if cfg.ReportPullingTimeout > 0 {
		c.ReportPullingTimeout = cfg.ReportPullingTimeout
	}
	if cfg.ReportMinRetryTime > 0 {
		c.ReportMinRetryTime = cfg.ReportMinRetryTime
	}
	c.EnableGlobalObfuscation = c.EnableGlobalObfuscation || cfg.EnableGlobalObfuscation
	c.DisableInsightsAlerts = c.DisableInsightsAlerts || cfg.DisableInsightsAlerts
}

func (c *Controller) mergeOCM(cfg *Controller) {
	if len(cfg.OCMConfig.SCAEndpoint) > 0 {
		c.OCMConfig.SCAEndpoint = cfg.OCMConfig.SCAEndpoint
	}
	if cfg.OCMConfig.SCAInterval > 0 {
		c.OCMConfig.SCAInterval = cfg.OCMConfig.SCAInterval
	}
	c.OCMConfig.SCADisabled = cfg.OCMConfig.SCADisabled

	if len(cfg.OCMConfig.ClusterTransferEndpoint) > 0 {
		c.OCMConfig.ClusterTransferEndpoint = cfg.OCMConfig.ClusterTransferEndpoint
	}
	if cfg.OCMConfig.ClusterTransferInterval > 0 {
		c.OCMConfig.ClusterTransferInterval = cfg.OCMConfig.ClusterTransferInterval
	}
}

func (c *Controller) mergeHTTP(cfg *Controller) {
	c.HTTPConfig = cfg.HTTPConfig
}

func (c *Controller) mergeInterval(cfg *Controller) {
	if cfg.Interval > 0 {
		c.Interval = cfg.Interval
	}
}

// ToController creates/updates a config Controller according to the Serialized config.
// Makes sure that the config is correct.
func ToController(s *Serialized, cfg *Controller) (*Controller, error) { // nolint: gocyclo, funlen
	if cfg == nil {
		cfg = &Controller{}
	}
	cfg.Report = s.Report
	cfg.StoragePath = s.StoragePath
	cfg.Endpoint = s.Endpoint
	cfg.ConditionalGathererEndpoint = s.ConditionalGathererEndpoint
	cfg.Impersonate = s.Impersonate
	cfg.EnableGlobalObfuscation = s.EnableGlobalObfuscation
	cfg.DisableInsightsAlerts = s.DisableInsightsAlerts

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

	if len(s.OCM.SCAEndpoint) > 0 {
		cfg.OCMConfig.SCAEndpoint = s.OCM.SCAEndpoint
	}
	cfg.OCMConfig.SCADisabled = s.OCM.SCADisabled

	if len(s.OCM.SCAInterval) > 0 {
		i, err := time.ParseDuration(s.OCM.SCAInterval)
		if err != nil {
			return nil, fmt.Errorf("OCM SCA interval must be a valid duration: %v", err)
		}
		cfg.OCMConfig.SCAInterval = i
	}
	if len(s.OCM.SCAEndpoint) > 0 {
		cfg.OCMConfig.ClusterTransferEndpoint = s.OCM.ClusterTransferEndpoint
	}
	if len(s.OCM.ClusterTransferInterval) > 0 {
		i, err := time.ParseDuration(s.OCM.ClusterTransferInterval)
		if err != nil {
			return nil, fmt.Errorf("OCM Cluster transfer interval must be a valid duration: %v", err)
		}
		cfg.OCMConfig.ClusterTransferInterval = i
	}
	return cfg, nil
}

// ToDisconnectedController creates/updates a config Controller according to the Serialized config.
// Makes sure that the config is correct, but only checks fields necessary for disconnected operation.
func ToDisconnectedController(s *Serialized, cfg *Controller) (*Controller, error) {
	if cfg == nil {
		cfg = &Controller{}
	}
	cfg.Report = s.Report
	cfg.StoragePath = s.StoragePath
	cfg.Impersonate = s.Impersonate
	cfg.EnableGlobalObfuscation = s.EnableGlobalObfuscation
	cfg.ConditionalGathererEndpoint = s.ConditionalGathererEndpoint
	cfg.DisableInsightsAlerts = s.DisableInsightsAlerts

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
