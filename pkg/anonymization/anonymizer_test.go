package anonymization

import (
	"context"
	"fmt"
	"net"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/insights/v1alpha2"

	networkv1 "github.com/openshift/api/network/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/record"
)

func Test_GetNextIP(t *testing.T) {
	type testCase struct {
		originalIP net.IP
		nextIP     net.IP
		mask       net.IPMask
		overflow   bool
	}
	testCases := []testCase{
		{
			originalIP: net.IPv4(127, 0, 0, 0),
			nextIP:     net.IPv4(127, 0, 0, 1),
			mask:       net.IPv4Mask(255, 255, 255, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 0, 1),
			nextIP:     net.IPv4(192, 168, 0, 2),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 0, 254),
			nextIP:     net.IPv4(192, 168, 0, 255),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 0, 255),
			nextIP:     net.IPv4(192, 168, 1, 0),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 255, 255),
			nextIP:     net.IPv4(192, 168, 0, 0),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   true,
		},
		{
			originalIP: net.IPv4(10, 0, 0, 54),
			nextIP:     net.IPv4(10, 0, 0, 55),
			mask:       net.IPv4Mask(255, 255, 255, 254),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(10, 0, 0, 55),
			nextIP:     net.IPv4(10, 0, 0, 54),
			mask:       net.IPv4Mask(255, 255, 255, 254),
			overflow:   true,
		},
		{
			originalIP: net.IPv4(255, 255, 255, 255),
			nextIP:     net.IPv4(255, 255, 255, 255),
			mask:       net.IPv4Mask(255, 255, 255, 255),
			overflow:   true,
		},
		{
			originalIP: net.IPv4(255, 255, 255, 255),
			nextIP:     net.IPv4(0, 0, 0, 0),
			mask:       net.IPv4Mask(0, 0, 0, 0),
			overflow:   false,
		},
		// IPv6
		{
			originalIP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			nextIP:     net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			mask:       net.IPMask{255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			overflow:   false,
		},
		// IPv6
		{
			originalIP: net.IP{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255},
			nextIP:     net.IP{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0},
			mask:       net.IPMask{255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			overflow:   false,
		},
	}

	for _, testCase := range testCases {
		nextIP, overflow := getNextIP(testCase.originalIP, testCase.mask)
		assert.True(
			t,
			nextIP.Equal(testCase.nextIP),
			"IP %v and %v are not equal",
			nextIP.String(),
			testCase.nextIP,
		)
		assert.Equal(t, overflow, testCase.overflow)
	}
}

func getAnonymizer(t *testing.T) *NetworkAnonymizer {
	clusterBaseDomain := "example.com"
	clusterConfigHost := "apiserver.com" // in HyperShift, API Server does not share base domain
	networks := []string{
		"127.0.0.0/8",
		"192.168.0.0/16",
	}
	mockConfigMapConfigurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			Obfuscation: config.Obfuscation{
				config.Networking,
			},
		},
	})
	networkAnonymizeBuilder := &NetworkAnonymizerBuilder{}
	networkAnonymizeBuilder.
		WithSensitiveValue(clusterBaseDomain, ClusterBaseDomainPlaceholder).
		WithSensitiveValue(clusterConfigHost, ClusterHostPlaceholder).
		WithConfigurator(mockConfigMapConfigurator).
		WithDataPolicies(v1alpha2.DataPolicyOptionObfuscateNetworking).
		WithNetworks(networks).
		WithSecretsClient(kubefake.NewSimpleClientset().CoreV1().Secrets(secretNamespace))
	networkAnonymizer, err := networkAnonymizeBuilder.Build()
	assert.NoError(t, err)

	return networkAnonymizer
}

func Test_Anonymizer(t *testing.T) {
	anonymizer := getAnonymizer(t)

	type testCase struct {
		before string
		after  string
	}

	nameTestCases := []testCase{
		{"node1.example.com", "node1.<CLUSTER_BASE_DOMAIN>"},
		{"api.example.com/test", "api.<CLUSTER_BASE_DOMAIN>/test"},
		{"https://example.apiserver.com:6443", "https://example.<CLUSTER_DOMAIN_HOST>:6443"},
	}
	dataTestCases := []testCase{
		{"api.example.com\n127.0.0.1  ", "api.<CLUSTER_BASE_DOMAIN>\n127.0.0.1  "},
		{"api.example.com\n127.0.0.128  ", "api.<CLUSTER_BASE_DOMAIN>\n127.0.0.2  "},
		{"127.0.0.1  ", "127.0.0.1  "},
		{"127.0.0.128  ", "127.0.0.2  "},
		{"192.168.1.15  ", "192.168.0.1  "},
		{"192.168.1.5  ", "192.168.0.2  "},
		{"192.168.1.255  ", "192.168.0.3  "},
		{"192.169.1.255  ", "0.0.0.0  "},
		{`{"key1": "val1", "key2": "127.0.0.128"'}`, `{"key1": "val1", "key2": "127.0.0.2"'}`},
		{`{"APIServerURL": "https://example.apiserver.com:6443"}`, `{"APIServerURL": "https://example.<CLUSTER_DOMAIN_HOST>:6443"}`},
	}

	for _, testCase := range nameTestCases {
		obfuscatedName, err := anonymizer.AnonymizeData(&record.MemoryRecord{
			Name: testCase.before,
		})

		assert.NoError(t, err)
		assert.Equal(t, testCase.after, obfuscatedName.Name)
	}

	for _, testCase := range dataTestCases {
		obfuscatedData, err := anonymizer.AnonymizeData(&record.MemoryRecord{
			Data: []byte(testCase.before),
		})
		tmp := string(obfuscatedData.Data)

		assert.NoError(t, err)
		assert.Equal(t, testCase.after, tmp)
	}
}

func Test_Anonymizer_TranslationTableTest(t *testing.T) {
	anonymizer := getAnonymizer(t)

	for i := 0; i < 254; i++ {
		obfuscatedData, err := anonymizer.AnonymizeData(&record.MemoryRecord{
			Data: []byte(fmt.Sprintf("192.168.0.%v", 255-i)),
		})

		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("192.168.0.%v", i+1), string(obfuscatedData.Data))
	}

	// 192.168.0.0 is the network address, we don't want to change it
	obfuscatedData, err := anonymizer.AnonymizeData(&record.MemoryRecord{
		Data: []byte("192.168.0.0"),
	})

	assert.NoError(t, err)
	assert.Equal(t, "192.168.0.0", string(obfuscatedData.Data))

	obfuscatedData, err = anonymizer.AnonymizeData(&record.MemoryRecord{
		Data: []byte("192.168.1.255"),
	})

	assert.NoError(t, err)
	assert.Equal(t, "192.168.0.255", string(obfuscatedData.Data))

	obfuscatedData, err = anonymizer.AnonymizeData(&record.MemoryRecord{
		Data: []byte("192.168.1.55"),
	})

	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.0", string(obfuscatedData.Data))

	obfuscatedData, err = anonymizer.AnonymizeData(&record.MemoryRecord{
		Data: []byte("192.168.1.56"),
	})

	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.1", string(obfuscatedData.Data))

	assert.Equal(t, 257, len(anonymizer.translationTable))
	anonymizer.ResetTranslationTable()
	assert.Equal(t, 0, len(anonymizer.translationTable))
}

func Test_Anonymizer_StoreTranslationTable(t *testing.T) {
	anonymizer := getAnonymizer(t)

	// Empty translation table == No call made to
	assert.Nil(t, anonymizer.StoreTranslationTable())

	// Mock the client to react/check Apply calls
	kube := kubefake.Clientset{}
	kube.Fake.AddReactor("create", "secrets",
		func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
			if createAction, ok := action.(clienttesting.CreateAction); ok {
				assert.Equal(t, secretNamespace, createAction.GetNamespace())
				assert.Equal(t, secretAPIVersion, createAction.GetResource().Version)
				var secret *corev1.Secret
				secret, ok = createAction.GetObject().(*corev1.Secret)
				if !ok {
					t.Errorf("Failed to convert sent Secret.")
				}
				return true, secret, nil
			}
			t.Errorf("Incorrect action, expected patch got %s", action)
			return false, nil, nil
		})
	anonymizer.secretsClient = kube.CoreV1().Secrets(secretNamespace)

	// Fill translation table
	for i := 0; i < 10; i++ {
		obfuscatedData, err := anonymizer.AnonymizeData(&record.MemoryRecord{
			Data: []byte(fmt.Sprintf("192.168.0.%v", 255-i)),
		})

		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("192.168.0.%v", i+1), string(obfuscatedData.Data))
	}
	// Store translation table, then check
	secret := anonymizer.StoreTranslationTable()
	for i := 0; i < 10; i++ {
		assert.Equal(t, secret.StringData[fmt.Sprintf("192.168.0.%v", 255-i)], fmt.Sprintf("192.168.0.%v", i+1))
	}
}

func TestNewAnonymizerFromConfigClient(t *testing.T) {
	const testClusterBaseDomain = "example.com"
	localhostCIDR := "127.0.0.0/8"
	_, localhostNet, err := net.ParseCIDR(localhostCIDR)
	assert.NoError(t, err)
	clusterNetworkCIDR := "55.44.0.0/16"
	_, net1, err := net.ParseCIDR(clusterNetworkCIDR)
	assert.NoError(t, err)
	serviceNetworkCIDR := "192.168.0.0/16"
	_, net2, err := net.ParseCIDR(serviceNetworkCIDR)
	assert.NoError(t, err)
	egressCIDR := "10.0.0.0/8"
	_, egressNet, err := net.ParseCIDR(egressCIDR)
	assert.NoError(t, err)

	tests := []struct {
		name               string
		dns                *configv1.DNS
		network            *configv1.Network
		hostsubnet         *networkv1.HostSubnet
		clusterConfigMap   *corev1.ConfigMap
		expectedSubnetInfo []subnetInformation
	}{
		{
			name: "Network config includes DNS, ExternalIP and HostSubnet exists",
			dns: &configv1.DNS{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       configv1.DNSSpec{BaseDomain: testClusterBaseDomain},
			},
			network: &configv1.Network{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.NetworkSpec{
					ClusterNetwork: []configv1.ClusterNetworkEntry{{CIDR: clusterNetworkCIDR}},
					ServiceNetwork: []string{serviceNetworkCIDR},
					ExternalIP:     &configv1.ExternalIPConfig{Policy: &configv1.ExternalIPPolicy{}},
				},
			},
			hostsubnet: &networkv1.HostSubnet{
				EgressCIDRs: []networkv1.HostSubnetEgressCIDR{networkv1.HostSubnetEgressCIDR(egressCIDR)},
			},
			clusterConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-config-v1"},
			},
			expectedSubnetInfo: []subnetInformation{
				{
					network: *localhostNet,
					lastIP:  net.IPv4(127, 0, 0, 0),
				},
				{
					network: *egressNet,
					lastIP:  net.IPv4(10, 0, 0, 0),
				},
				{
					network: *net1,
					lastIP:  net.IPv4(55, 44, 0, 0),
				},
				{
					network: *net2,
					lastIP:  net.IPv4(192, 168, 0, 0),
				},
			},
		},
		{
			name: "Network config includes DNS, ExternalIP and HostSubnet is nil",
			dns: &configv1.DNS{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       configv1.DNSSpec{BaseDomain: testClusterBaseDomain},
			},
			network: &configv1.Network{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.NetworkSpec{
					ClusterNetwork: []configv1.ClusterNetworkEntry{{CIDR: clusterNetworkCIDR}},
					ServiceNetwork: []string{serviceNetworkCIDR},
					ExternalIP:     &configv1.ExternalIPConfig{Policy: &configv1.ExternalIPPolicy{}},
				},
			},
			hostsubnet: nil,
			clusterConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-config-v1"},
			},
			expectedSubnetInfo: []subnetInformation{
				{
					network: *localhostNet,
					lastIP:  net.IPv4(127, 0, 0, 0),
				},
				{
					network: *egressNet,
					// when hostsubnet doesn't exist then OVN egress CIDR 192.168.126.0/18
					// is added
					lastIP: net.IPv4(192, 168, 64, 0),
				},
				{
					network: *net1,
					lastIP:  net.IPv4(55, 44, 0, 0),
				},
				{
					network: *net2,
					lastIP:  net.IPv4(192, 168, 0, 0),
				},
			},
		},
		{
			name: "Network config includes DNS, HostSubnet but ExternalIP is nil",
			dns: &configv1.DNS{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec:       configv1.DNSSpec{BaseDomain: testClusterBaseDomain},
			},
			network: &configv1.Network{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.NetworkSpec{
					ClusterNetwork: []configv1.ClusterNetworkEntry{{CIDR: clusterNetworkCIDR}},
					ServiceNetwork: []string{serviceNetworkCIDR},
					ExternalIP:     nil,
				},
			},
			hostsubnet: &networkv1.HostSubnet{
				EgressCIDRs: []networkv1.HostSubnetEgressCIDR{networkv1.HostSubnetEgressCIDR(egressCIDR)},
			},
			clusterConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-config-v1"},
			},
			expectedSubnetInfo: []subnetInformation{
				{
					network: *localhostNet,
					lastIP:  net.IPv4(127, 0, 0, 0),
				},
				{
					network: *egressNet,
					lastIP:  net.IPv4(10, 0, 0, 0),
				},
				{
					network: *net1,
					lastIP:  net.IPv4(55, 44, 0, 0),
				},
				{
					network: *net2,
					lastIP:  net.IPv4(192, 168, 0, 0),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := kubefake.NewSimpleClientset()
			coreClient := kubeClient.CoreV1()
			networkClient := networkfake.NewSimpleClientset().NetworkV1()
			configClient := configfake.NewSimpleClientset().ConfigV1()

			mockConfigMapConfigurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					Obfuscation: config.Obfuscation{
						config.Networking,
					},
				},
			})
			ctx := context.Background()
			_, err := configClient.DNSes().Create(ctx, tt.dns, metav1.CreateOptions{})
			assert.NoError(t, err)

			_, err = configClient.Networks().Create(ctx, tt.network, metav1.CreateOptions{})
			assert.NoError(t, err)

			_, err = coreClient.ConfigMaps("kube-system").Create(ctx, tt.clusterConfigMap, metav1.CreateOptions{})
			assert.NoError(t, err)

			_, err = configClient.Infrastructures().Create(ctx,
				&configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
				metav1.CreateOptions{})
			assert.NoError(t, err)

			if tt.hostsubnet != nil {
				_, err = networkClient.HostSubnets().Create(ctx, tt.hostsubnet, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			anonymizer, err := NewNetworkAnonymizerFromConfigClient(
				context.Background(),
				kubeClient,
				kubeClient,
				configClient,
				networkClient,
				mockConfigMapConfigurator,
				[]v1alpha2.DataPolicyOption{v1alpha2.DataPolicyOptionObfuscateNetworking},
				make(map[string]string),
			)
			assert.NoError(t, err)
			assert.NotNil(t, anonymizer)

			_, exists := anonymizer.sensitiveValues[testClusterBaseDomain]
			assert.True(t, exists)
			assert.Empty(t, anonymizer.translationTable)
			assert.NotNil(t, anonymizer.ipNetworkRegex)
			assert.NotNil(t, anonymizer.secretsClient)

			err = anonymizer.readNetworkConfigs()
			assert.NoError(t, err)
			assert.Equal(t, len(tt.expectedSubnetInfo), len(anonymizer.networks))
			// the networks are already sorted in anonymizer
			for i, subnetInfo := range anonymizer.networks {
				expectedSubnetInfo := tt.expectedSubnetInfo[i]
				assert.Equal(t, expectedSubnetInfo.network.Network(), subnetInfo.network.Network())
				assert.Equal(t, expectedSubnetInfo.lastIP.String(), subnetInfo.lastIP.String())
			}
		})
	}
}

func TestAddParsedDomainToMap(t *testing.T) {
	tests := []struct {
		name          string
		address       string
		placeholder   string
		expectedKey   string
		expectedValue string
		expectError   bool
	}{
		{
			name:          "valid URL with hostname",
			address:       "https://api.example.com:6443",
			placeholder:   "<CLUSTER_HOST>",
			expectedKey:   "api.example.com",
			expectedValue: "<CLUSTER_HOST>",
			expectError:   false,
		},
		{
			name:          "hostname only",
			address:       "api.example.com",
			placeholder:   "<CLUSTER_HOST>",
			expectedKey:   "api.example.com",
			expectedValue: "<CLUSTER_HOST>",
			expectError:   false,
		},
		{
			name:          "IP address with scheme",
			address:       "https://192.168.1.1:6443",
			placeholder:   "<CLUSTER_HOST>",
			expectedKey:   "192.168.1.1",
			expectedValue: "<CLUSTER_HOST>",
			expectError:   false,
		},
		{
			name:          "invalid URL with special characters",
			address:       "ht\ttp://invalid",
			placeholder:   "<CLUSTER_HOST>",
			expectedKey:   "",
			expectedValue: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainMap := make(map[string]string)
			err := addParsedDomainToMap(tt.address, domainMap, tt.placeholder)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, domainMap, 1)
			assert.Equal(t, tt.expectedValue, domainMap[tt.expectedKey])
		})
	}
}

func TestAddAPIDomainsForAnonymization(t *testing.T) {
	tests := []struct {
		name            string
		clientHosts     []string
		infrastructure  *configv1.Infrastructure
		expectedDomains map[string]string
		expectError     bool
	}{
		{
			name:        "infrastructure with valid API server URL",
			clientHosts: []string{"host1.example.com", "host2.example.com"},
			infrastructure: &configv1.Infrastructure{
				Status: configv1.InfrastructureStatus{
					APIServerURL: "https://api.cluster.example.com:6443",
				},
			},
			expectedDomains: map[string]string{
				"api.cluster.example.com": ClusterHostPlaceholder,
			},
			expectError: false,
		},
		{
			name:        "infrastructure with empty API server URL - uses client hosts",
			clientHosts: []string{"host1.example.com", "host2.example.com"},
			infrastructure: &configv1.Infrastructure{
				Status: configv1.InfrastructureStatus{
					APIServerURL: "",
				},
			},
			expectedDomains: map[string]string{
				"host1.example.com": ClusterHostPlaceholder,
				"host2.example.com": ClusterHostPlaceholder,
			},
			expectError: false,
		},
		{
			name:           "nil infrastructure - uses client hosts",
			clientHosts:    []string{"host1.example.com", "host2.example.com"},
			infrastructure: nil,
			expectedDomains: map[string]string{
				"host1.example.com": ClusterHostPlaceholder,
				"host2.example.com": ClusterHostPlaceholder,
			},
			expectError: false,
		},
		{
			name:            "empty client hosts with nil infrastructure",
			clientHosts:     []string{},
			infrastructure:  nil,
			expectedDomains: map[string]string{},
			expectError:     false,
		},
		{
			name:           "mixed client hosts - URLs and hostnames",
			clientHosts:    []string{"https://host1.example.com:6443", "host2.example.com", "192.168.1.1"},
			infrastructure: nil,
			expectedDomains: map[string]string{
				"host1.example.com": ClusterHostPlaceholder,
				"host2.example.com": ClusterHostPlaceholder,
				"192.168.1.1":       ClusterHostPlaceholder,
			},
			expectError: false,
		},
		{
			name:            "client hosts with invalid URL",
			clientHosts:     []string{"ht\ttp://invalid"},
			infrastructure:  nil,
			expectedDomains: map[string]string{},
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainMap := make(map[string]string)
			err := addAPIDomainsForAnonymization(tt.clientHosts, tt.infrastructure, domainMap)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expectedDomains), len(domainMap))
			for expectedKey, expectedValue := range tt.expectedDomains {
				assert.Equal(t, expectedValue, domainMap[expectedKey], "Domain %s should map to %s", expectedKey, expectedValue)
			}
		})
	}
}
