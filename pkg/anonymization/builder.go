package anonymization

import (
	"regexp"
	"slices"
	"strings"

	insightsv1 "github.com/openshift/api/insights/v1"
	v1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	k8snet "k8s.io/utils/net"
)

type AnonBuilder struct {
	anon     Anonymizer
	networks []string
}

// WithSensitiveValue adds terms that are obfuscated by the anonymizer in the records.
// It works as a key-value map, where all instances of 'value' are replaced by 'placeholder'.
func (b *AnonBuilder) WithSensitiveValue(value, placeholder string) *AnonBuilder {
	v := strings.TrimSpace(value)
	if v == "" {
		return b
	}
	b.makeMapIfNil()
	b.anon.sensitiveValues[v] = placeholder
	return b
}

func (b *AnonBuilder) WithConfigClient(configClient v1.ConfigV1Interface) *AnonBuilder {
	b.anon.configClient = configClient
	return b
}

func (b *AnonBuilder) WithConfigurator(configurator configobserver.Interface) *AnonBuilder {
	b.anon.configurator = configurator
	return b
}

func (b *AnonBuilder) WithDataPolicies(dataPolicy ...insightsv1.DataPolicyOption) *AnonBuilder {
	b.anon.dataPolicy = ""

	if slices.Contains(dataPolicy, insightsv1.DataPolicyOptionObfuscateNetworking) {
		b.anon.dataPolicy = insightsv1.DataPolicyOptionObfuscateNetworking
	}

	return b
}

func (b *AnonBuilder) WithKubeClient(kubeClient kubernetes.Interface) *AnonBuilder {
	b.anon.gatherKubeClient = kubeClient
	return b
}

func (b *AnonBuilder) WithNetworkClient(networkClient networkv1client.NetworkV1Interface) *AnonBuilder {
	b.anon.networkClient = networkClient
	return b
}

func (b *AnonBuilder) WithNetworks(networks []string) *AnonBuilder {
	b.networks = networks
	return b
}

func (b *AnonBuilder) WithRunningInCluster(runningInCluster bool) *AnonBuilder {
	b.anon.runningInCluster = runningInCluster
	return b
}

func (b *AnonBuilder) WithSecretsClient(client corev1client.SecretInterface) *AnonBuilder {
	b.anon.secretsClient = client
	return b
}

func (b *AnonBuilder) Build() (*Anonymizer, error) {
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

func (b *AnonBuilder) makeMapIfNil() {
	if b.anon.sensitiveValues == nil {
		b.anon.sensitiveValues = make(map[string]string)
	}
}
