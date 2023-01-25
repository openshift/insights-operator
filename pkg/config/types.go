package config

import (
	"time"

	"k8s.io/klog/v2"
)

// InsightsConfigurationSerialized is a type representing Insights
// Operator configuration values in JSON/YAML and it is when decoding
// the content of the "insights-config" config map.
type InsightsConfigurationSerialized struct {
	DataReporting DataReportingSerialized `json:"dataReporting"`
}

type DataReportingSerialized struct {
	Interval         string `json:"interval,omitempty"`
	UploadEndpoint   string `json:"uploadEndpoint,omitempty"`
	DownloadEndpoint string `json:"downloadEndpoint,omitempty"`
}

// InsightsConfiguration is a type representing actual Insights
// Operator configuration options and is used in the code base
// to make the configuration available.
type InsightsConfiguration struct {
	DataReporting DataReporting
}

// DataReporting is a type including all
// the configuration options related to Insights data gathering,
// upload of the data and download of the corresponding Insights analysis report.
type DataReporting struct {
	Interval         time.Duration
	UploadEndpoint   string
	DownloadEndpoint string
}

// ToConfig reads and pareses the actual serialized configuration from "InsightsConfigurationSerialized"
// and returns the "InsightsConfiguration".
func (i InsightsConfigurationSerialized) ToConfig() *InsightsConfiguration {
	ic := &InsightsConfiguration{
		DataReporting: DataReporting{
			UploadEndpoint:   i.DataReporting.UploadEndpoint,
			DownloadEndpoint: i.DataReporting.DownloadEndpoint,
		},
	}
	if i.DataReporting.Interval != "" {
		interval, err := time.ParseDuration(i.DataReporting.Interval)
		if err != nil {
			klog.Errorf("Cannot parse interval time duration: %v. Using default value 2h", err)
			return ic
		}
		ic.DataReporting.Interval = interval
	}
	return ic
}
