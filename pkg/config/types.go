package config

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"
)

const (
	// defines default frequency of the data gathering
	defaultGatherFrequency = 2 * time.Hour
	// defines default frequency of the SCA download
	defaultSCAFfrequency = 8 * time.Hour
	// defines default frequency of the Cluster Transfer download
	defaultClusterTransferFrequency = 12 * time.Hour
)

// InsightsConfigurationSerialized is a type representing Insights
// Operator configuration values in JSON/YAML and it is when decoding
// the content of the "insights-config" config map.
type InsightsConfigurationSerialized struct {
	DataReporting   DataReportingSerialized   `json:"dataReporting"`
	Alerting        AlertingSerialized        `json:"alerting,omitempty"`
	SCA             SCASerialized             `json:"sca,omitempty"`
	ClusterTransfer ClusterTransferSerialized `json:"clusterTransfer,omitempty"`
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

type ClusterTransferSerialized struct {
	Interval string `json:"interval,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// InsightsConfiguration is a type representing actual Insights
// Operator configuration options and is used in the code base
// to make the configuration available.
type InsightsConfiguration struct {
	DataReporting   DataReporting
	Alerting        Alerting
	SCA             SCA
	ClusterTransfer ClusterTransfer
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

// ClusterTransfer is a helper type for configuring Insights
// cluster transfer (ownership) feature
type ClusterTransfer struct {
	Interval time.Duration
	Endpoint string
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
		ClusterTransfer: ClusterTransfer{
			Endpoint: i.ClusterTransfer.Endpoint,
		},
	}
	if i.DataReporting.Interval != "" {
		ic.DataReporting.Interval = parseInterval(i.DataReporting.Interval, defaultGatherFrequency)
	}

	if i.SCA.Interval != "" {
		ic.SCA.Interval = parseInterval(i.SCA.Interval, defaultSCAFfrequency)
	}
	if i.ClusterTransfer.Interval != "" {
		ic.ClusterTransfer.Interval = parseInterval(i.ClusterTransfer.Interval, defaultClusterTransferFrequency)
	}

	return ic
}

// parseInterval tries to parse the "interval" string as time duration and if there is an error
// or negative time value then the provided default time duration is used
func parseInterval(interval string, defaultValue time.Duration) time.Duration {
	durationInt, err := time.ParseDuration(interval)
	if err != nil {
		klog.Errorf("Cannot parse interval time duration: %v. Using default value %s", err, defaultValue)
		return defaultValue
	}
	if durationInt <= 0 {
		durationInt = defaultValue
	}
	return durationInt
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
