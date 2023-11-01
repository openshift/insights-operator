package config

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

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
		SCA: SCA{
			Endpoint: i.SCA.Endpoint,
		},
		ClusterTransfer: ClusterTransfer{
			Endpoint: i.ClusterTransfer.Endpoint,
		},
		Proxy: Proxy{
			HTTPProxy:  i.Proxy.HTTPProxy,
			HTTPSProxy: i.Proxy.HTTPSProxy,
			NoProxy:    i.Proxy.NoProxy,
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

	if i.Alerting.Disabled != "" {
		ic.Alerting.Disabled = strings.EqualFold(i.Alerting.Disabled, "true")
	}

	if i.SCA.Disabled != "" {
		ic.SCA.Disabled = strings.EqualFold(i.SCA.Disabled, "true")
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
	obfuscation=%s,
	sca=%v,
	alerting=%v,
	clusterTransfer=%v,
	proxy=%v`,
		i.DataReporting.Interval,
		i.DataReporting.UploadEndpoint,
		i.DataReporting.StoragePath,
		i.DataReporting.DownloadEndpoint,
		i.DataReporting.ConditionalGathererEndpoint,
		i.DataReporting.Obfuscation,
		i.SCA,
		i.Alerting,
		i.ClusterTransfer,
		i.Proxy,
	)
	return s
}
