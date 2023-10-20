package config

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"
)

const (
	defaultGatherPeriod = 2 * time.Hour
	defaultSCAPeriod    = 8 * time.Hour
)

// InsightsConfigurationSerialized is a type representing Insights
// Operator configuration values in JSON/YAML and it is when decoding
// the content of the "insights-config" config map.
type InsightsConfigurationSerialized struct {
	DataReporting DataReportingSerialized `json:"dataReporting"`
	Alerting      AlertingSerialized      `json:"alerting,omitempty"`
	SCA           SCASerialized           `json:"sca,omitempty"`
}

type DataReportingSerialized struct {
	Interval                    string      `json:"interval,omitempty"`
	UploadEndpoint              string      `json:"uploadEndpoint,omitempty"`
	DownloadEndpoint            string      `json:"downloadEndpoint,omitempty"`
	DownloadEndpointTechPreview string      `json:"downloadEndpointTechPreview,omitempty"`
	StoragePath                 string      `json:"storagePath,omitempty"`
	ConditionalGathererEndpoint string      `json:"conditionalGathererEndpoint,omitempty"`
	ProcessingStatusEndpoint    string      `json:"processingStatusEndpoint,omitempty"`
	Obfuscation                 Obfuscation `json:"obfuscation,omitempty"`
}

type AlertingSerialized struct {
	Disabled bool `json:"disabled,omitempty"`
}

type SCASerialized struct {
	Disabled bool   `json:"disabled,omitempty"`
	Interval string `json:"interval,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// InsightsConfiguration is a type representing actual Insights
// Operator configuration options and is used in the code base
// to make the configuration available.
type InsightsConfiguration struct {
	DataReporting DataReporting
	Alerting      Alerting
	SCA           SCA
}

// DataReporting is a type including all
// the configuration options related to Insights data gathering,
// upload of the data and download of the corresponding Insights analysis report.
type DataReporting struct {
	Enabled                     bool
	Interval                    time.Duration
	UploadEndpoint              string
	DownloadEndpoint            string
	DownloadEndpointTechPreview string
	StoragePath                 string
	ConditionalGathererEndpoint string
	ReportPullingDelay          time.Duration
	ProcessingStatusEndpoint    string
	Obfuscation                 Obfuscation
}

// Alerting is a helper type for configuring Insights alerting
// options
type Alerting struct {
	Disabled bool
}

// SCA is a helper type for configuring periodical download/check
// of the SimpleContentAcccess entitlements
type SCA struct {
	Disabled bool
	Interval time.Duration
	Endpoint string
}

const (
	Networking    ObfuscationValue = "networking"
	WorkloadNames ObfuscationValue = "workload_names"
)

type ObfuscationValue string

type Obfuscation []ObfuscationValue

// ToConfig reads and pareses the actual serialized configuration from "InsightsConfigurationSerialized"
// and returns the "InsightsConfiguration".
func (i *InsightsConfigurationSerialized) ToConfig() *InsightsConfiguration {
	ic := &InsightsConfiguration{
		DataReporting: DataReporting{
			UploadEndpoint:              i.DataReporting.UploadEndpoint,
			DownloadEndpoint:            i.DataReporting.DownloadEndpoint,
			DownloadEndpointTechPreview: i.DataReporting.DownloadEndpointTechPreview,
			StoragePath:                 i.DataReporting.StoragePath,
			ConditionalGathererEndpoint: i.DataReporting.ConditionalGathererEndpoint,
			ProcessingStatusEndpoint:    i.DataReporting.ProcessingStatusEndpoint,
			Obfuscation:                 i.DataReporting.Obfuscation,
		},
		Alerting: Alerting{
			Disabled: i.Alerting.Disabled,
		},
		SCA: SCA{
			Disabled: i.SCA.Disabled,
			Endpoint: i.SCA.Endpoint,
		},
	}
	if i.DataReporting.Interval != "" {
		interval, err := time.ParseDuration(i.DataReporting.Interval)
		if err != nil {
			klog.Errorf("Cannot parse interval time duration: %v. Using default value %s", err, defaultGatherPeriod)
		}
		if interval <= 0 {
			interval = defaultGatherPeriod
		}
		ic.DataReporting.Interval = interval
	}

	if i.SCA.Interval != "" {
		interval, err := time.ParseDuration(i.SCA.Interval)
		if err != nil {
			klog.Errorf("Cannot parse interval time duration: %v. Using default value %s", err, defaultSCAPeriod)
		}
		if interval <= 0 {
			interval = defaultSCAPeriod
		}
		ic.SCA.Interval = interval
	}

	return ic
}

func (i *InsightsConfiguration) String() string {
	s := fmt.Sprintf(`upload_interval=%s, 
	upload_endpoint=%s,
	storage_path=%s, 
	download_endpoint=%s, 
	conditional_gatherer_endpoint=%s,
	obfuscation=%s`,
		i.DataReporting.Interval,
		i.DataReporting.UploadEndpoint,
		i.DataReporting.StoragePath,
		i.DataReporting.DownloadEndpoint,
		i.DataReporting.ConditionalGathererEndpoint,
		i.DataReporting.Obfuscation,
	)
	return s
}
