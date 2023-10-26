package configobserver

import (
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestMergeStatically(t *testing.T) {
	tests := []struct {
		name           string
		configCM       *v1.ConfigMap
		legacyConfig   config.Controller
		expectedConfig *config.InsightsConfiguration
	}{
		{
			name:     "No config map exists - legacy config is used",
			configCM: nil,
			legacyConfig: config.Controller{
				Report:                      true,
				StoragePath:                 "/foo/bar/",
				Endpoint:                    "http://testing.here",
				ReportEndpoint:              "http://reportendpoint.here",
				Interval:                    2 * time.Hour,
				ConditionalGathererEndpoint: "http://conditionalendpoint.here",
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "http://testing.here",
					StoragePath:                 "/foo/bar/",
					DownloadEndpoint:            "http://reportendpoint.here",
					Interval:                    2 * time.Hour,
					ConditionalGathererEndpoint: "http://conditionalendpoint.here",
				},
			},
		},
		{
			name: "Config map exists and overrides legacy config",
			configCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      insightsConfigMapName,
					Namespace: "openshift-insights",
				},
				Data: map[string]string{
					"config.yaml": `
dataReporting:
  interval: 1h
  uploadEndpoint: https://overriden.upload/endpoint
  storagePath: /var/lib/test
  downloadEndpoint: https://overriden.download/endpoint
  conditionalGathererEndpoint: https://overriden.conditional/endpoint
  processingStatusEndpoint: https://overriden.status/endpoint
  downloadEndpointTechPreview: https://overriden.downloadtechpreview/endpoint`,
				},
			},
			legacyConfig: config.Controller{
				Report:                      true,
				StoragePath:                 "/foo/bar/",
				Endpoint:                    "http://testing.here",
				ReportEndpoint:              "http://reportendpoint.here",
				Interval:                    2 * time.Hour,
				ConditionalGathererEndpoint: "http://conditionalendpoint.here",
				ProcessingStatusEndpoint:    "http://statusendpoint.here",
				ReportEndpointTechPreview:   "http://downloadtpendpoint.here",
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "https://overriden.upload/endpoint",
					StoragePath:                 "/var/lib/test",
					DownloadEndpoint:            "https://overriden.download/endpoint",
					Interval:                    1 * time.Hour,
					ConditionalGathererEndpoint: "https://overriden.conditional/endpoint",
					ProcessingStatusEndpoint:    "https://overriden.status/endpoint",
					DownloadEndpointTechPreview: "https://overriden.downloadtechpreview/endpoint",
				},
			},
		},
		{
			name: "Config map cannot override \"Report\" bool attribute",
			configCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      insightsConfigMapName,
					Namespace: "openshift-insights",
				},
				Data: map[string]string{
					"config.yaml": `
dataReporting:
  enabled: true
  uploadEndpoint: https://overriden.upload/endpoint`,
				},
			},
			legacyConfig: config.Controller{
				Report:   false,
				Endpoint: "http://testing.here",
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:        false,
					UploadEndpoint: "https://overriden.upload/endpoint",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cs *kubefake.Clientset
			if tt.configCM != nil {
				cs = kubefake.NewSimpleClientset(tt.configCM)
			} else {
				cs = kubefake.NewSimpleClientset()
			}
			mockSecretConf := config.NewMockSecretConfigurator(&tt.legacyConfig)
			staticAggregator := NewStaticConfigAggregator(mockSecretConf, cs)

			testConfig := staticAggregator.Config()
			assert.Equal(t, tt.expectedConfig, testConfig)
		})
	}
}

func TestMergeUsingInformer(t *testing.T) {
	tests := []struct {
		name           string
		configFromInf  config.InsightsConfiguration
		legacyConfig   config.Controller
		expectedConfig *config.InsightsConfiguration
	}{
		{
			name:          "No config map exists - legacy config is used",
			configFromInf: config.InsightsConfiguration{},
			legacyConfig: config.Controller{
				Report:                      true,
				StoragePath:                 "/foo/bar/",
				Endpoint:                    "http://testing.here",
				ReportEndpoint:              "http://reportendpoint.here",
				Interval:                    2 * time.Hour,
				ConditionalGathererEndpoint: "http://conditionalendpoint.here",
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "http://testing.here",
					StoragePath:                 "/foo/bar/",
					DownloadEndpoint:            "http://reportendpoint.here",
					Interval:                    2 * time.Hour,
					ConditionalGathererEndpoint: "http://conditionalendpoint.here",
				},
			},
		},
		{
			name: "Config map exists and overrides legacy config",
			configFromInf: config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Interval:                    1 * time.Hour,
					UploadEndpoint:              "https://overriden.upload/endpoint",
					StoragePath:                 "/var/lib/test",
					DownloadEndpoint:            "https://overriden.download/endpoint",
					ConditionalGathererEndpoint: "https://overriden.conditional/endpoint",
				},
			},
			legacyConfig: config.Controller{
				Report:                      true,
				StoragePath:                 "/foo/bar/",
				Endpoint:                    "http://testing.here",
				ReportEndpoint:              "http://reportendpoint.here",
				Interval:                    2 * time.Hour,
				ConditionalGathererEndpoint: "http://conditionalendpoint.here",
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "https://overriden.upload/endpoint",
					StoragePath:                 "/var/lib/test",
					DownloadEndpoint:            "https://overriden.download/endpoint",
					Interval:                    1 * time.Hour,
					ConditionalGathererEndpoint: "https://overriden.conditional/endpoint",
				},
			},
		},
		{
			name: "Config map cannot override \"Report\" bool attribute",
			configFromInf: config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:        true,
					UploadEndpoint: "https://overriden.upload/endpoint",
				},
			},
			legacyConfig: config.Controller{
				Report:   false,
				Endpoint: "http://testing.here",
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:        false,
					UploadEndpoint: "https://overriden.upload/endpoint",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSecretConf := config.NewMockSecretConfigurator(&tt.legacyConfig)
			mockConfigMapInf := NewMockConfigMapInformer(&tt.configFromInf)
			informerAggregator := NewConfigAggregator(mockSecretConf, mockConfigMapInf)

			testConfig := informerAggregator.Config()
			assert.Equal(t, tt.expectedConfig, testConfig)
		})
	}
}

type MockConfigMapInformer struct {
	factory.Controller
	config *config.InsightsConfiguration
}

func NewMockConfigMapInformer(cfg *config.InsightsConfiguration) *MockConfigMapInformer {
	return &MockConfigMapInformer{
		config: cfg,
	}
}

func (m *MockConfigMapInformer) Config() *config.InsightsConfiguration {
	return m.config
}

func (m *MockConfigMapInformer) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
	return nil, nil
}
