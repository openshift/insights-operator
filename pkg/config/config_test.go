package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToConfig(t *testing.T) {
	tests := []struct {
		name             string
		serializedConfig InsightsConfigurationSerialized
		config           *InsightsConfiguration
	}{
		{
			name: "basic test",
			serializedConfig: InsightsConfigurationSerialized{
				DataReporting: DataReportingSerialized{
					Interval:       "5m",
					UploadEndpoint: "test.upload.endpoint/v1",
					StoragePath:    "/tmp/test/path",
					Obfuscation: Obfuscation{
						Networking,
						WorkloadNames,
					},
				},
				SCA: SCASerialized{
					Disabled: "true",
					Interval: "12h",
					Endpoint: "test.sca.endpoint",
				},
				ClusterTransfer: ClusterTransferSerialized{
					Interval: "14h",
				},
				Alerting: AlertingSerialized{
					Disabled: "false",
				},
			},
			config: &InsightsConfiguration{
				DataReporting: DataReporting{
					Interval:       5 * time.Minute,
					UploadEndpoint: "test.upload.endpoint/v1",
					StoragePath:    "/tmp/test/path",
					Obfuscation: Obfuscation{
						Networking,
						WorkloadNames,
					},
				},
				SCA: SCA{
					Disabled: true,
					Interval: 12 * time.Hour,
					Endpoint: "test.sca.endpoint",
				},
				ClusterTransfer: ClusterTransfer{
					Interval: 14 * time.Hour,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testConfig := tt.serializedConfig.ToConfig()
			assert.Equal(t, tt.config, testConfig)
		})
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		name             string
		intervalString   string
		defaultValue     time.Duration
		expectedInterval time.Duration
	}{
		{
			name:             "basic test with meaningful interval value",
			intervalString:   "1h",
			defaultValue:     30 * time.Minute,
			expectedInterval: 1 * time.Hour,
		},
		{
			name:             "interval cannot be parsed",
			intervalString:   "not a duration",
			defaultValue:     30 * time.Minute,
			expectedInterval: 30 * time.Minute,
		},
		{
			name:             "interval is negative duration",
			intervalString:   "-10m",
			defaultValue:     30 * time.Minute,
			expectedInterval: 30 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval := parseInterval(tt.intervalString, tt.defaultValue)
			assert.Equal(t, tt.expectedInterval, interval)
		})
	}
}
