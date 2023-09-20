package config

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// nolint: funlen
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		ctrl           Controller
		obj            map[string]interface{}
		expectedOutput Controller
		err            error
	}{
		{
			name: "controller defaults are overwritten by the serialized config",
			ctrl: Controller{
				Endpoint:             "default-endpoint",
				Report:               false,
				Interval:             5 * time.Minute,
				StoragePath:          "default-storage-path",
				ReportEndpoint:       "default-report-endpoint",
				ReportPullingDelay:   30 * time.Second,
				ReportMinRetryTime:   60 * time.Second,
				ReportPullingTimeout: 2 * time.Minute,
				OCMConfig: OCMConfig{
					SCAInterval:             1 * time.Hour,
					SCAEndpoint:             "default-sca-endpoint",
					ClusterTransferEndpoint: "default-ct-endpoint",
					ClusterTransferInterval: 24 * time.Hour,
				},
			},
			obj: map[string]interface{}{
				"report":      true,
				"interval":    "2h",
				"endpoint":    "real-endpoint",
				"storagePath": "/tmp/insights-operator",
				"pull_report": map[string]interface{}{
					"delay":     "1m",
					"min_retry": "5m",
					"endpoint":  "real-pull-report-endpoint",
					"timeout":   "4m",
				},
				"ocm": map[string]interface{}{
					"scaInterval":             "8h",
					"scaEndpoint":             "real-sca-endpoint",
					"clusterTransferEndpoint": "real-ct-endpoint",
					"clusterTransferInterval": "12h",
				},
			},
			expectedOutput: Controller{
				Report:               true,
				Interval:             2 * time.Hour,
				Endpoint:             "real-endpoint",
				StoragePath:          "/tmp/insights-operator",
				ReportEndpoint:       "real-pull-report-endpoint",
				ReportPullingDelay:   1 * time.Minute,
				ReportMinRetryTime:   5 * time.Minute,
				ReportPullingTimeout: 4 * time.Minute,
				OCMConfig: OCMConfig{
					SCAInterval:             8 * time.Hour,
					SCAEndpoint:             "real-sca-endpoint",
					ClusterTransferEndpoint: "real-ct-endpoint",
					ClusterTransferInterval: 12 * time.Hour,
				},
			},
			err: nil,
		},
		{
			name:           "interval cannot be empty",
			ctrl:           Controller{},
			obj:            map[string]interface{}{},
			expectedOutput: Controller{},
			err:            fmt.Errorf("interval must be a non-negative duration"),
		},
		{
			name: "interval must be valid duration",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "notnumber",
			},
			expectedOutput: Controller{
				Interval: 0,
			},
			err: fmt.Errorf("interval must be a valid duration: time: invalid duration \"notnumber\""),
		},
		{
			name: "delay cannot be empty",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "2h",
			},
			expectedOutput: Controller{
				Interval: 2 * time.Hour,
			},
			err: fmt.Errorf("delay must be a non-negative duration"),
		},
		{
			name: "min_retry cannot be empty",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "2h",
				"pull_report": map[string]interface{}{
					"delay": "1m",
				},
			},
			expectedOutput: Controller{
				Interval:           2 * time.Hour,
				ReportPullingDelay: 1 * time.Minute,
			},
			err: fmt.Errorf("min_retry must be a non-negative duration"),
		},
		{
			name: "timeout cannot be empty",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "2h",
				"pull_report": map[string]interface{}{
					"delay":     "1m",
					"min_retry": "2m",
				},
			},
			expectedOutput: Controller{
				Interval:           2 * time.Hour,
				ReportPullingDelay: 1 * time.Minute,
				ReportMinRetryTime: 2 * time.Minute,
			},
			err: fmt.Errorf("timeout must be a non-negative duration"),
		},
		{
			name: "storagePath cannot be empty",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "2h",
				"pull_report": map[string]interface{}{
					"delay":     "1m",
					"min_retry": "2m",
					"timeout":   "5m",
				},
			},
			expectedOutput: Controller{
				Interval:             2 * time.Hour,
				ReportPullingDelay:   1 * time.Minute,
				ReportMinRetryTime:   2 * time.Minute,
				ReportPullingTimeout: 5 * time.Minute,
			},
			err: fmt.Errorf("storagePath must point to a directory where snapshots can be stored"),
		},
		{
			name: "SCA interval must be valid duration",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "2h",
				"pull_report": map[string]interface{}{
					"delay":     "1m",
					"min_retry": "2m",
					"timeout":   "5m",
				},
				"storagePath": "test/path",
				"ocm": map[string]interface{}{
					"scaInterval": "not-duration",
				},
			},
			expectedOutput: Controller{
				Interval:             2 * time.Hour,
				ReportPullingDelay:   1 * time.Minute,
				ReportMinRetryTime:   2 * time.Minute,
				ReportPullingTimeout: 5 * time.Minute,
				StoragePath:          "test/path",
			},
			err: fmt.Errorf("OCM SCA interval must be a valid duration: time: invalid duration \"not-duration\""),
		},
		{
			name: "SCA interval must be valid duration",
			ctrl: Controller{},
			obj: map[string]interface{}{
				"interval": "2h",
				"pull_report": map[string]interface{}{
					"delay":     "1m",
					"min_retry": "2m",
					"timeout":   "5m",
				},
				"storagePath": "test/path",
				"ocm": map[string]interface{}{
					"clusterTransferInterval": "not-duration",
				},
			},
			expectedOutput: Controller{
				Interval:             2 * time.Hour,
				ReportPullingDelay:   1 * time.Minute,
				ReportMinRetryTime:   2 * time.Minute,
				ReportPullingTimeout: 5 * time.Minute,
				StoragePath:          "test/path",
			},
			err: fmt.Errorf("OCM Cluster transfer interval must be a valid duration: time: invalid duration \"not-duration\""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := LoadConfig(tt.ctrl, tt.obj, ToController)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.expectedOutput, output)
		})
	}
}
