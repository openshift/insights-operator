package config

import (
	"time"
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
	Proxy           ProxySeriazlied           `json:"proxy,omitempty"`
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
	Disabled string `json:"disabled,omitempty"`
}

type SCASerialized struct {
	Disabled string `json:"disabled,omitempty"`
	Interval string `json:"interval,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type ClusterTransferSerialized struct {
	Interval string `json:"interval,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type ProxySeriazlied struct {
	HTTPProxy  string `json:"httpProxy,omitempty"`
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	NoProxy    string `json:"noProxy,omitempty"`
}

// InsightsConfiguration is a type representing actual Insights
// Operator configuration options and is used in the code base
// to make the configuration available.
type InsightsConfiguration struct {
	DataReporting   DataReporting
	Alerting        Alerting
	SCA             SCA
	ClusterTransfer ClusterTransfer
	Proxy           Proxy
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

// Proxy is a helper type for configuring connection proxy
type Proxy struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

const (
	Networking    ObfuscationValue = "networking"
	WorkloadNames ObfuscationValue = "workload_names"
)

type ObfuscationValue string

type Obfuscation []ObfuscationValue
