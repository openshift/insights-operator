package anonymization

import (
	"regexp"
	"strings"

	"github.com/openshift/api/insights/v1alpha1"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	k8snet "k8s.io/utils/net"
)

type AnonBuilder struct {
	anon     Anonymizer
	networks []string
}

func (b *AnonBuilder) WithClusterBaseDomain(baseDomain string) *AnonBuilder {
	b.anon.clusterBaseDomain = strings.TrimSpace(baseDomain)
	return b
}

func (b *AnonBuilder) WithConfigurator(configurator configobserver.Interface) *AnonBuilder {
	b.anon.configurator = configurator
	return b
}

func (b *AnonBuilder) WithDataPolicy(dataPolicy v1alpha1.DataPolicy) *AnonBuilder {
	b.anon.dataPolicy = dataPolicy
	return b
}

func (b *AnonBuilder) WithNetworks(networks []string) *AnonBuilder {
	b.networks = networks
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

	b.anon.ipNetworkRegex = regexp.MustCompile(Ipv4AddressOrNetworkRegex)
	b.anon.networks = networksInformation
	b.anon.translationTable = make(map[string]string)

	return &b.anon, nil
}
