package configobserver

import (
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/openshift/insights-operator/pkg/config"
)

func TestConfigSecret_loadCredentials(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want *ConfigSecret
	}{
		{
			name: "Load credentials",
			data: map[string][]byte{
				"username": []byte("user"),
				"password": []byte("xxxxxx"),
			},
			want: &ConfigSecret{Controller: config.Controller{
				Report:   false,
				Username: "user",
				Password: "xxxxxx",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &ConfigSecret{Controller: config.Controller{}}
			got.loadCredentials(tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadCredentials() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSecret_loadEndpoint(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want *ConfigSecret
	}{
		{
			name: "Load HTTP Config",
			data: map[string][]byte{"endpoint": []byte("http://endpoint")},
			want: &ConfigSecret{Controller: config.Controller{
				Report:   false,
				Endpoint: "http://endpoint",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &ConfigSecret{Controller: config.Controller{}}
			got.loadEndpoint(tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadEndpoint() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSecret_loadHTTP(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want *ConfigSecret
	}{
		{
			name: "Load HTTP Config",
			data: map[string][]byte{
				"httpProxy":  []byte("http://proxy"),
				"httpsProxy": []byte("https://proxy"),
				"noProxy":    []byte("true"),
			},
			want: &ConfigSecret{Controller: config.Controller{
				Report: false,
				HTTPConfig: config.HTTPConfig{
					HTTPProxy:  "http://proxy",
					HTTPSProxy: "https://proxy",
					NoProxy:    "true",
				},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &ConfigSecret{Controller: config.Controller{}}
			got.loadHTTP(tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadHTTP() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSecret_loadOCM(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want *ConfigSecret
	}{
		{
			name: "Load OCM Config",
			data: map[string][]byte{
				"scaEndpoint":     []byte("http://endpoint"),
				"scaInterval":     []byte("2h"),
				"scaPullDisabled": []byte("false"),
			},
			want: &ConfigSecret{Controller: config.Controller{
				Report: false,
				OCMConfig: config.OCMConfig{
					SCAInterval: 2 * time.Hour,
					SCAEndpoint: "http://endpoint",
					SCADisabled: false,
				},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &ConfigSecret{Controller: config.Controller{}}
			got.loadOCM(tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadOCM() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSecret_loadReport(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want *ConfigSecret
	}{
		{
			name: "Load Report Config",
			data: map[string][]byte{
				"reportEndpoint":       []byte("http://endpoint"),
				"reportPullingDelay":   []byte("1h"),
				"reportPullingTimeout": []byte("1h"),
				"reportMinRetryTime":   []byte("30m"),
			},
			want: &ConfigSecret{Controller: config.Controller{
				Report:               false,
				ReportEndpoint:       "http://endpoint",
				ReportPullingDelay:   1 * time.Hour,
				ReportPullingTimeout: 1 * time.Hour,
				ReportMinRetryTime:   30 * time.Minute,
			}},
		},
		{
			name: "Load Report Config (missing pulling delay)",
			data: map[string][]byte{
				"reportEndpoint":       []byte("http://endpoint"),
				"reportPullingTimeout": []byte("1h"),
				"reportMinRetryTime":   []byte("30m"),
			},
			want: &ConfigSecret{Controller: config.Controller{
				Report:               false,
				ReportEndpoint:       "http://endpoint",
				ReportPullingDelay:   time.Duration(-1),
				ReportPullingTimeout: 1 * time.Hour,
				ReportMinRetryTime:   30 * time.Minute,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &ConfigSecret{Controller: config.Controller{}}
			got.loadReport(tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadReport() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadSecretConfig(t *testing.T) {
	tests := []struct {
		name    string
		secret  *v1.Secret
		want    config.Controller
		wantErr bool
	}{
		{
			name: "Can load from secret",
			secret: &v1.Secret{
				Data: map[string][]byte{
					"username":        []byte("user"),
					"password":        []byte("xxxxxx"),
					"endpoint":        []byte("http://endpoint"),
					"noProxy":         []byte("no-proxy"),
					"reportEndpoint":  []byte("http://report"),
					"scaPullDisabled": []byte("false"),
				},
			},
			want: config.Controller{
				Report:             true,
				Endpoint:           "http://endpoint",
				ReportEndpoint:     "http://report",
				Username:           "user",
				Password:           "xxxxxx",
				ReportPullingDelay: time.Duration(-1),
				HTTPConfig: config.HTTPConfig{
					NoProxy: "no-proxy",
				},
				OCMConfig: config.OCMConfig{
					SCADisabled: false,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadSecretConfig(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadSecretConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadSecretConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}
