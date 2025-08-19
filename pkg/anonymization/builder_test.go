package anonymization

import (
	"testing"

	insightsv1 "github.com/openshift/api/insights/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	v1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_AnonBuilder(t *testing.T) {
	type testCase struct {
		name             string
		builder          *AnonBuilder
		sensitiveValues  map[string]string
		configClient     v1.ConfigV1Interface
		configurator     configobserver.Interface
		dataPolicy       insightsv1.DataPolicyOption
		kubeClient       kubernetes.Interface
		networkClient    networkv1client.NetworkV1Interface
		networks         []string
		runningInCluster bool
		secretsClient    corev1client.SecretInterface
	}

	fakeCfgClient := configfake.NewSimpleClientset().ConfigV1()
	fakeConfigurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{})
	fakeKubeClient := kubefake.NewSimpleClientset()
	fakeNetworkClient := networkfake.NewSimpleClientset().NetworkV1()
	fakeSecretsClient := kubefake.NewSimpleClientset().CoreV1().Secrets("mock")

	testCases := []testCase{
		{
			name:    "Basic builder need no settings to create a valid anonymizer",
			builder: getBuilderInstance(),
		},
		{
			name:            "method 'AddSensitiveValue' sets values on the anonymizer instance",
			builder:         getBuilderInstance().WithSensitiveValue("mock", "xxxx"),
			sensitiveValues: map[string]string{"mock": "xxxx"},
		},
		{
			name:         "method 'WithConfigClient' sets the client on the anonymizer instance",
			builder:      getBuilderInstance().WithConfigClient(fakeCfgClient),
			configClient: fakeCfgClient,
		},
		{
			name:         "method 'WithConfigurator' sets the configurator on the anonymizer instance",
			builder:      getBuilderInstance().WithConfigurator(fakeConfigurator),
			configurator: fakeConfigurator,
		},
		{
			name:       "method 'WithDataPolicy' sets the policy on the anonymizer instance",
			builder:    getBuilderInstance().WithDataPolicies(insightsv1.DataPolicyOptionObfuscateNetworking),
			dataPolicy: insightsv1.DataPolicyOptionObfuscateNetworking,
		},
		{
			name:       "method 'WithKubeClient' sets the client on the anonymizer instance",
			builder:    getBuilderInstance().WithKubeClient(fakeKubeClient),
			kubeClient: fakeKubeClient,
		},
		{
			name:          "method 'WithNetworkClient' sets the client on the anonymizer instance",
			builder:       getBuilderInstance().WithNetworkClient(fakeNetworkClient),
			networkClient: fakeNetworkClient,
		},
		{
			name:     "method 'WithNetworks' sets the networks on the anonymizer instance",
			builder:  getBuilderInstance().WithNetworks([]string{"127.0.0.0/8", "192.168.0.0/16"}),
			networks: []string{"127.0.0.0/8", "192.168.0.0/16"},
		},
		{
			name:             "method 'WithRunningInCluster' sets the value on the anonymizer instance",
			builder:          getBuilderInstance().WithRunningInCluster(true),
			runningInCluster: true,
		},
		{
			name:          "method 'WithSecretsClient' sets the client on the anonymizer instance",
			builder:       getBuilderInstance().WithSecretsClient(fakeSecretsClient),
			secretsClient: fakeSecretsClient,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When
			test, err := tc.builder.Build()

			// Assert
			assert.NoError(t, err)
			assert.IsType(t, Anonymizer{}, *test)
			if tc.sensitiveValues != nil {
				assert.EqualValues(t, tc.sensitiveValues, test.sensitiveValues)
			}
			assert.Equal(t, tc.configClient, test.configClient)
			assert.Equal(t, tc.configurator, test.configurator)
			assert.Equal(t, tc.dataPolicy, test.dataPolicy)
			assert.Equal(t, tc.kubeClient, test.gatherKubeClient)
			assert.Equal(t, tc.networkClient, test.networkClient)
			assert.Equal(t, len(tc.networks), len(test.networks))
			if tc.runningInCluster {
				assert.True(t, test.runningInCluster)
			}
			assert.Equal(t, tc.secretsClient, test.secretsClient)
		})
	}
}

func getBuilderInstance() *AnonBuilder {
	return &AnonBuilder{}
}
