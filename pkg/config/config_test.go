package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/yaml"
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

func Test_ObfuscationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		yamlInput   string
		expectedObf Obfuscation
		expectError bool
	}{
		{
			name:        "empty string treated as empty array",
			yamlInput:   "obfuscation: ''",
			expectedObf: Obfuscation{},
			expectError: false,
		},
		{
			name:        "empty array",
			yamlInput:   "obfuscation: []",
			expectedObf: Obfuscation{},
			expectError: false,
		},
		{
			name:        "single valid value as string - networking",
			yamlInput:   "obfuscation: networking",
			expectedObf: Obfuscation{Networking},
			expectError: false,
		},
		{
			name:        "single valid value as string - workload_names",
			yamlInput:   "obfuscation: workload_names",
			expectedObf: Obfuscation{WorkloadNames},
			expectError: false,
		},
		{
			name:        "invalid single value as string - defaults to empty",
			yamlInput:   "obfuscation: invalid_value",
			expectedObf: Obfuscation{},
			expectError: false,
		},
		{
			name:        "array with single value",
			yamlInput:   "obfuscation:\n  - networking",
			expectedObf: Obfuscation{Networking},
			expectError: false,
		},
		{
			name:        "array with multiple values",
			yamlInput:   "obfuscation:\n  - networking\n  - workload_names",
			expectedObf: Obfuscation{Networking, WorkloadNames},
			expectError: false,
		},
		{
			name:        "array with multiple values - 1 invalid",
			yamlInput:   "obfuscation:\n  - networking\n  - invalid_value",
			expectedObf: Obfuscation{Networking},
			expectError: false,
		},
		{
			name:        "field omitted",
			yamlInput:   "someOtherField: value",
			expectedObf: nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a struct that contains Obfuscation field to test unmarshaling
			type testStruct struct {
				Obfuscation Obfuscation `json:"obfuscation,omitempty"`
			}

			var result testStruct
			err := yaml.Unmarshal([]byte(tt.yamlInput), &result)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedObf, result.Obfuscation)
			}
		})
	}
}
