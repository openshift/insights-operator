package controller

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

func Test_prepareGatherConfigs(t *testing.T) {
	tests := []struct {
		name                    string
		protoKubeConfig         *rest.Config
		kubeConfig              *rest.Config
		impersonate             string
		prometheusTokenEnvValue string
		expectImpersonation     bool
		expectInsecureMetrics   bool
		expectInsecureAlerts    bool
	}{
		{
			name: "with impersonation",
			protoKubeConfig: &rest.Config{
				Host: "https://api.test-cluster:6443",
			},
			kubeConfig: &rest.Config{
				Host: "https://api.test-cluster:6443",
			},
			impersonate:             "system:serviceaccount:openshift-insights:gather",
			prometheusTokenEnvValue: "",
			expectImpersonation:     true,
			expectInsecureMetrics:   false,
			expectInsecureAlerts:    false,
		},
		{
			name: "with prometheus token",
			protoKubeConfig: &rest.Config{
				Host: "https://api.test-cluster:6443",
			},
			kubeConfig: &rest.Config{
				Host: "https://api.test-cluster:6443",
			},
			impersonate:             "",
			prometheusTokenEnvValue: "test-token-67890",
			expectImpersonation:     false,
			expectInsecureMetrics:   true,
			expectInsecureAlerts:    true,
		},
		{
			name: "prometheus token with whitespace is trimmed",
			protoKubeConfig: &rest.Config{
				Host: "https://api.test-cluster:6443",
			},
			kubeConfig: &rest.Config{
				Host: "https://api.test-cluster:6443",
			},
			impersonate:             "",
			prometheusTokenEnvValue: "  test-token-with-spaces  \n",
			expectImpersonation:     false,
			expectInsecureMetrics:   true,
			expectInsecureAlerts:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			if tt.prometheusTokenEnvValue != "" {
				os.Setenv(insecurePrometheusTokenEnvVariable, tt.prometheusTokenEnvValue)
				defer os.Unsetenv(insecurePrometheusTokenEnvVariable)
			} else {
				os.Unsetenv(insecurePrometheusTokenEnvVariable)
			}

			// Call the function
			gatherProto, gatherKube, metricsGather, alertsGather := prepareGatherConfigs(
				tt.protoKubeConfig,
				tt.kubeConfig,
				tt.impersonate,
			)

			assert.NotNil(t, gatherProto)
			assert.NotNil(t, gatherKube)
			assert.NotNil(t, metricsGather)
			assert.NotNil(t, alertsGather)

			assert.Empty(t, tt.protoKubeConfig.Impersonate.UserName)
			assert.Empty(t, tt.kubeConfig.Impersonate.UserName)

			if tt.expectImpersonation {
				assert.Equal(t, tt.impersonate, gatherProto.Impersonate.UserName)
				assert.Equal(t, tt.impersonate, gatherKube.Impersonate.UserName)
			} else {
				assert.Empty(t, gatherProto.Impersonate.UserName)
				assert.Empty(t, gatherKube.Impersonate.UserName)
			}

			assert.Equal(t, metricHost, metricsGather.Host)
			assert.Equal(t, "/", metricsGather.APIPath)
			assert.NotNil(t, metricsGather.GroupVersion)
			assert.NotNil(t, metricsGather.NegotiatedSerializer)

			if tt.expectInsecureMetrics {
				assert.True(t, metricsGather.Insecure)
				assert.NotEmpty(t, metricsGather.BearerToken)
				assert.Empty(t, metricsGather.CAFile)
				assert.Empty(t, metricsGather.CAData)
			} else {
				assert.False(t, metricsGather.Insecure)
				assert.Empty(t, metricsGather.BearerToken)
				assert.Equal(t, metricCAFile, metricsGather.CAFile)
			}

			assert.Equal(t, alertManagerHost, alertsGather.Host)
			assert.Equal(t, "/", alertsGather.APIPath)
			assert.NotNil(t, alertsGather.GroupVersion)
			assert.NotNil(t, alertsGather.NegotiatedSerializer)

			if tt.expectInsecureAlerts {
				assert.True(t, alertsGather.Insecure)
				assert.NotEmpty(t, alertsGather.BearerToken)
				assert.Empty(t, alertsGather.CAFile)
				assert.Empty(t, alertsGather.CAData)
			} else {
				assert.False(t, alertsGather.Insecure)
				assert.Empty(t, alertsGather.BearerToken)
				assert.Equal(t, metricCAFile, alertsGather.CAFile)
			}
		})
	}
}

func Test_createGatherConfig(t *testing.T) {
	tests := []struct {
		name           string
		kubeConfig     *rest.Config
		configHost     string
		token          string
		expectInsecure bool
	}{
		{
			name: "without token",
			kubeConfig: &rest.Config{
				Host:    "https://api.test-cluster:6443",
				APIPath: "/api",
				TLSClientConfig: rest.TLSClientConfig{
					CAFile: "/original/ca.crt",
					CAData: []byte("original-ca-data"),
				},
			},
			configHost:     "https://prometheus.example.com:9091",
			token:          "",
			expectInsecure: false,
		},
		{
			name: "with token",
			kubeConfig: &rest.Config{
				Host:    "https://api.test-cluster:6443",
				APIPath: "/api",
				TLSClientConfig: rest.TLSClientConfig{
					CAFile: "/original/ca.crt",
					CAData: []byte("original-ca-data"),
				},
			},
			configHost:     "https://prometheus.example.com:9091",
			token:          "test-bearer-token",
			expectInsecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatherConfig := createGatherConfig(tt.kubeConfig, tt.configHost, tt.token)

			assert.NotNil(t, gatherConfig)
			assert.Equal(t, "https://api.test-cluster:6443", tt.kubeConfig.Host)
			assert.Equal(t, "/api", tt.kubeConfig.APIPath)

			assert.Equal(t, tt.configHost, gatherConfig.Host)
			assert.Equal(t, "/", gatherConfig.APIPath)
			assert.NotNil(t, gatherConfig.GroupVersion)
			assert.Equal(t, &schema.GroupVersion{}, gatherConfig.GroupVersion)
			assert.Equal(t, scheme.Codecs, gatherConfig.NegotiatedSerializer)

			if tt.expectInsecure {
				assert.True(t, gatherConfig.Insecure)
				assert.Equal(t, tt.token, gatherConfig.BearerToken)
				assert.Empty(t, gatherConfig.CAFile)
				assert.Empty(t, gatherConfig.CAData)
			} else {
				assert.False(t, gatherConfig.Insecure)
				assert.Empty(t, gatherConfig.BearerToken)
				assert.Equal(t, metricCAFile, gatherConfig.CAFile)
			}
		})
	}
}
