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

func (d *DataReporting) String() string {
	s := fmt.Sprintf(`
		interval: %s,
		uploadEndpoint: %s,
		storagePath: %s,
		downloadEndpoint: %s, 
		conditionalGathererEndpoint: %s,
		obfuscation: %s`,
		d.Interval,
		d.UploadEndpoint,
		d.StoragePath,
		d.DownloadEndpoint,
		d.ConditionalGathererEndpoint,
		d.Obfuscation)
	return s
}

func (s *SCA) String() string {
	str := fmt.Sprintf(`
		disabled: %v,
		endpoint: %s,
		interval: %s`,
		s.Disabled,
		s.Endpoint,
		s.Interval)
	return str
}

func (a *Alerting) String() string {
	s := fmt.Sprintf(`
		disabled: %v`, a.Disabled)
	return s
}

func (p *Proxy) String() string {
	s := fmt.Sprintf(`
		httpProxy: %s,
		httpsProxy: %s,
		noProxy: %s`,
		p.HTTPProxy,
		p.HTTPSProxy,
		p.NoProxy)
	return s
}

func (c *ClusterTransfer) String() string {
	s := fmt.Sprintf(`
		endpoint: %s,
		interval: %s`,
		c.Endpoint,
		c.Interval)
	return s
}

func (i *InsightsConfiguration) String() string {
	s := fmt.Sprintf(`
	dataReporting:%s
	sca:%s
	alerting:%s
	clusterTransfer:%s
	proxy:%s`,
		i.DataReporting.String(),
		i.SCA.String(),
		i.Alerting.String(),
		i.ClusterTransfer.String(),
		i.Proxy.String(),
	)
	return s
}
