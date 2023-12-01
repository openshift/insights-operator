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
				EnableGlobalObfuscation:     true,
				DisableInsightsAlerts:       true,
				OCMConfig: config.OCMConfig{
					SCAInterval: 5 * time.Hour,
					SCAEndpoint: "test.sca.endpoint",
					SCADisabled: true,
				},
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "http://testing.here",
					StoragePath:                 "/foo/bar/",
					DownloadEndpoint:            "http://reportendpoint.here",
					Interval:                    2 * time.Hour,
					ConditionalGathererEndpoint: "http://conditionalendpoint.here",
					Obfuscation:                 config.Obfuscation{config.Networking},
				},
				SCA: config.SCA{
					Interval: 5 * time.Hour,
					Endpoint: "test.sca.endpoint",
					Disabled: true,
				},
				Alerting: config.Alerting{
					Disabled: true,
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
  downloadEndpointTechPreview: https://overriden.downloadtechpreview/endpoint
  obfuscation:
  - workload_names
alerting:
  disabled: true
sca:
  disabled: true
  endpoint: updated.sca.endpoint
clusterTransfer:
  interval: 12h
  endpoint: cluster.transfer.endpoint.overriden`,
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
				EnableGlobalObfuscation:     true,
				DisableInsightsAlerts:       false,
				OCMConfig: config.OCMConfig{
					SCAInterval:             5 * time.Hour,
					SCAEndpoint:             "test.sca.endpoint",
					SCADisabled:             false,
					ClusterTransferEndpoint: "cluster.transfer.endpoint",
					ClusterTransferInterval: 10 * time.Hour,
				},
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
					Obfuscation:                 config.Obfuscation{config.Networking, config.WorkloadNames},
				},
				Alerting: config.Alerting{
					Disabled: true,
				},
				SCA: config.SCA{
					Disabled: true,
					Interval: 5 * time.Hour,
					Endpoint: "updated.sca.endpoint",
				},
				ClusterTransfer: config.ClusterTransfer{
					Interval: 12 * time.Hour,
					Endpoint: "cluster.transfer.endpoint.overriden",
				},
			},
		},
		{
			name: "Config map cannot override \"report\" bool attribute",
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
		{
			name: "Empty config also overrides the legacy config with zero values",
			configCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      insightsConfigMapName,
					Namespace: "openshift-insights",
				},
				Data: map[string]string{
					"config.yaml": ``,
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
				EnableGlobalObfuscation:     true,
				DisableInsightsAlerts:       true,
				OCMConfig: config.OCMConfig{
					SCAInterval:             5 * time.Hour,
					SCAEndpoint:             "test.sca.endpoint",
					SCADisabled:             true,
					ClusterTransferEndpoint: "cluster.transfer.endpoint",
					ClusterTransferInterval: 10 * time.Hour,
				},
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "http://testing.here",
					StoragePath:                 "/foo/bar/",
					DownloadEndpoint:            "http://reportendpoint.here",
					Interval:                    2 * time.Hour,
					ConditionalGathererEndpoint: "http://conditionalendpoint.here",
					ProcessingStatusEndpoint:    "http://statusendpoint.here",
					DownloadEndpointTechPreview: "http://downloadtpendpoint.here",
					Obfuscation:                 config.Obfuscation{config.Networking},
				},
				Alerting: config.Alerting{
					Disabled: false,
				},
				SCA: config.SCA{
					Disabled: false,
					Interval: 5 * time.Hour,
					Endpoint: "test.sca.endpoint",
				},
				ClusterTransfer: config.ClusterTransfer{
					Interval: 10 * time.Hour,
					Endpoint: "cluster.transfer.endpoint",
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
		configMap      *v1.ConfigMap
		legacyConfig   config.Controller
		expectedConfig *config.InsightsConfiguration
	}{
		{
			name:      "No config map exists - legacy config is used",
			configMap: nil,
			legacyConfig: config.Controller{
				Report:                      true,
				StoragePath:                 "/foo/bar/",
				Endpoint:                    "http://testing.here",
				ReportEndpoint:              "http://reportendpoint.here",
				Interval:                    2 * time.Hour,
				ConditionalGathererEndpoint: "http://conditionalendpoint.here",
				EnableGlobalObfuscation:     true,
				DisableInsightsAlerts:       true,
				HTTPConfig: config.HTTPConfig{
					HTTPProxy:  "http://test.proxy",
					HTTPSProxy: "https://test.proxy",
					NoProxy:    "https://no.proxy",
				},
				OCMConfig: config.OCMConfig{
					SCAInterval: 5 * time.Hour,
					SCAEndpoint: "test.sca.endpoint",
					SCADisabled: true,
				},
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "http://testing.here",
					StoragePath:                 "/foo/bar/",
					DownloadEndpoint:            "http://reportendpoint.here",
					Interval:                    2 * time.Hour,
					ConditionalGathererEndpoint: "http://conditionalendpoint.here",
					Obfuscation:                 config.Obfuscation{config.Networking},
				},
				Alerting: config.Alerting{
					Disabled: true,
				},
				Proxy: config.Proxy{
					HTTPProxy:  "http://test.proxy",
					HTTPSProxy: "https://test.proxy",
					NoProxy:    "https://no.proxy",
				},
				SCA: config.SCA{
					Disabled: true,
					Endpoint: "test.sca.endpoint",
					Interval: 5 * time.Hour,
				},
			},
		},
		{
			name: "Config map exists and overrides legacy config",
			configMap: &v1.ConfigMap{
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
  downloadEndpointTechPreview: https://overriden.downloadtechpreview/endpoint
  obfuscation:
  - workload_names
alerting:
  disabled: true
sca:
  disabled: true
  endpoint: updated.sca.endpoint
  interval: 8h
clusterTransfer:
  interval: 12h
  endpoint: cluster.transfer.endpoint.overriden
proxy:
  httpProxy: http://test.proxy.updated
  httpsProxy: https://test.proxy.updated
  noProxy: https://no.proxy.updated`,
				},
			},
			legacyConfig: config.Controller{
				Report:                      true,
				StoragePath:                 "/foo/bar/",
				Endpoint:                    "http://testing.here",
				ReportEndpoint:              "http://reportendpoint.here",
				Interval:                    2 * time.Hour,
				ConditionalGathererEndpoint: "http://conditionalendpoint.here",
				EnableGlobalObfuscation:     true,
				DisableInsightsAlerts:       false,
				HTTPConfig: config.HTTPConfig{
					HTTPProxy:  "http://test.proxy",
					HTTPSProxy: "https://test.proxy",
					NoProxy:    "https://no.proxy",
				},
				OCMConfig: config.OCMConfig{
					SCAInterval:             4 * time.Hour,
					SCAEndpoint:             "endpoint",
					SCADisabled:             true,
					ClusterTransferEndpoint: "cluster.transfer.endpoint",
					ClusterTransferInterval: 10 * time.Hour,
				},
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:                     true,
					UploadEndpoint:              "https://overriden.upload/endpoint",
					StoragePath:                 "/var/lib/test",
					DownloadEndpoint:            "https://overriden.download/endpoint",
					DownloadEndpointTechPreview: "https://overriden.downloadtechpreview/endpoint",
					ProcessingStatusEndpoint:    "https://overriden.status/endpoint",
					Interval:                    1 * time.Hour,
					ConditionalGathererEndpoint: "https://overriden.conditional/endpoint",
					Obfuscation:                 config.Obfuscation{config.Networking, config.WorkloadNames},
				},
				Alerting: config.Alerting{
					Disabled: true,
				},
				Proxy: config.Proxy{
					HTTPProxy:  "http://test.proxy.updated",
					HTTPSProxy: "https://test.proxy.updated",
					NoProxy:    "https://no.proxy.updated",
				},
				SCA: config.SCA{
					Disabled: true,
					Endpoint: "updated.sca.endpoint",
					Interval: 8 * time.Hour,
				},
				ClusterTransfer: config.ClusterTransfer{
					Interval: 12 * time.Hour,
					Endpoint: "cluster.transfer.endpoint.overriden",
				},
			},
		},
		{
			name: "Config map cannot override \"Report\" bool attribute",
			configMap: &v1.ConfigMap{
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
		{
			name: "Empty config also overrides the legacy config with zero values",
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      insightsConfigMapName,
					Namespace: "openshift-insights",
				},
				Data: map[string]string{
					"config.yaml": ``,
				},
			},
			legacyConfig: config.Controller{
				Report:                  true,
				Endpoint:                "http://testing.here",
				ReportEndpoint:          "http://reportendpoint.here",
				Interval:                2 * time.Hour,
				EnableGlobalObfuscation: true,
				DisableInsightsAlerts:   true,
				OCMConfig: config.OCMConfig{
					SCAInterval: 5 * time.Hour,
					SCAEndpoint: "test.sca.endpoint",
					SCADisabled: true,
				},
			},
			expectedConfig: &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Enabled:          true,
					UploadEndpoint:   "http://testing.here",
					DownloadEndpoint: "http://reportendpoint.here",
					Interval:         2 * time.Hour,
					Obfuscation:      config.Obfuscation{config.Networking},
				},
				Alerting: config.Alerting{
					Disabled: false, // this was not provide in the empty config map and zero value (false) is used
				},
				SCA: config.SCA{
					Disabled: false, // this was not provide in the empty config map and zero value (false) is used
					Endpoint: "test.sca.endpoint",
					Interval: 5 * time.Hour,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSecretConf := config.NewMockSecretConfigurator(&tt.legacyConfig)
			mockConfigMapInf, err := NewMockConfigMapInformer(tt.configMap)
			assert.NoError(t, err)
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

func NewMockConfigMapInformer(cm *v1.ConfigMap) (*MockConfigMapInformer, error) {
	if cm == nil {
		return &MockConfigMapInformer{
			config: nil,
		}, nil
	}

	cfg, err := readConfigAndDecode(cm)
	if err != nil {
		return nil, err
	}
	return &MockConfigMapInformer{
		config: cfg,
	}, nil
}

func (m *MockConfigMapInformer) Config() *config.InsightsConfiguration {
	return m.config
}

func (m *MockConfigMapInformer) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
	return nil, nil
}
