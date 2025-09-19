package anonymization

import (
	"regexp"
	"slices"
	"strings"

	"github.com/openshift/api/insights/v1alpha2"
	v1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	k8snet "k8s.io/utils/net"
)

type NetworkAnonymizerBuilder struct {
	anon     NetworkAnonymizer
	networks []string
}

// WithSensitiveValue adds terms that are obfuscated by the anonymizer in the records.
// It works as a key-value map, where all instances of 'value' are replaced by 'placeholder'.
func (b *NetworkAnonymizerBuilder) WithSensitiveValue(value, placeholder string) *NetworkAnonymizerBuilder {
	v := strings.TrimSpace(value)
	if v == "" {
		return b
	}
	b.makeMapIfNil()
	b.anon.sensitiveValues[v] = placeholder
	return b
}

func (b *NetworkAnonymizerBuilder) WithConfigClient(configClient v1.ConfigV1Interface) *NetworkAnonymizerBuilder {
	b.anon.configClient = configClient
	return b
}

func (b *NetworkAnonymizerBuilder) WithConfigurator(configurator configobserver.Interface) *NetworkAnonymizerBuilder {
	b.anon.configurator = configurator
	return b
}

func (b *NetworkAnonymizerBuilder) WithDataPolicies(dataPolicy ...v1alpha2.DataPolicyOption) *NetworkAnonymizerBuilder {
	b.anon.dataPolicy = ""

	if slices.Contains(dataPolicy, v1alpha2.DataPolicyOptionObfuscateNetworking) {
		b.anon.dataPolicy = v1alpha2.DataPolicyOptionObfuscateNetworking
	}

	return b
}

func (b *NetworkAnonymizerBuilder) WithKubeClient(kubeClient kubernetes.Interface) *NetworkAnonymizerBuilder {
	b.anon.gatherKubeClient = kubeClient
	return b
}

func (b *NetworkAnonymizerBuilder) WithNetworkClient(networkClient networkv1client.NetworkV1Interface) *NetworkAnonymizerBuilder {
	b.anon.networkClient = networkClient
	return b
}

func (b *NetworkAnonymizerBuilder) WithNetworks(networks []string) *NetworkAnonymizerBuilder {
	b.networks = networks
	return b
}

func (b *NetworkAnonymizerBuilder) WithRunningInCluster(runningInCluster bool) *NetworkAnonymizerBuilder {
	b.anon.runningInCluster = runningInCluster
	return b
}

func (b *NetworkAnonymizerBuilder) WithSecretsClient(client corev1client.SecretInterface) *NetworkAnonymizerBuilder {
	b.anon.secretsClient = client
	return b
}

func (b *NetworkAnonymizerBuilder) Build() (*NetworkAnonymizer, error) {
	cidrs, err := k8snet.ParseCIDRs(b.networks)
	if err != nil {
		return nil, err
	}

	var networksInformation []subnetInformation
	for _, network := range cidrs {
		lastIP := network.IP
		networksInformation = append(networksInformation, subnetInformation{
			network: *network,
			lastIP:  lastIP,
		})
	}

	b.makeMapIfNil()
	b.anon.ipNetworkRegex = regexp.MustCompile(Ipv4AddressOrNetworkRegex)
	b.anon.networks = networksInformation
	b.anon.translationTable = make(map[string]string)

	return &b.anon, nil
}

func (b *NetworkAnonymizerBuilder) makeMapIfNil() {
	if b.anon.sensitiveValues == nil {
		b.anon.sensitiveValues = make(map[string]string)
	}
}
