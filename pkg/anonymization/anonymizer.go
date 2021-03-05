// Package anonymization provides Anonymizer which is used to anonymize sensitive data. At the moment,
// anonymization is applied to all the data before storing it in the archive(see AnonymizeMemoryRecordFunction).
// If you enable it in the config, the following data will be anonymized:
//   - cluster base domain. For example, if the cluster base domain is `openshift.example.com`,
//     all the occurrences of this keyword will be replaced with `<CLUSTER_BASE_DOMAIN>`,
//     `cluster-api.openshift.example.com` will become `cluster-api.<CLUSTER_BASE_DOMAIN>`
//   - IPv4 addresses. Using a config client, it retrieves cluster networks and uses them to anonymize IP addresses
//     preserving subnet information. For example, if you have the following networks in your cluster:
//     "10.128.0.0/14", "172.30.0.0/16", "127.0.0.1/8"(added by default) the anonymization will handle the IPs like this:
//       - 10.128.0.0 -> 10.128.0.0
//       - 10.128.0.1 -> 10.128.0.0
//       - 10.129.0.0 -> 10.128.0.0
//       - 172.30.0.5 -> 172.30.0.0
//       - 127.0.0.1 -> 127.0.0.0
//       - 10.0.134.130 -> 0.0.0.0  // ip doesn't match any subnet

package anonymization

import (
	"bytes"
	"context"
	"net"
	"regexp"
	"strings"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	k8snet "k8s.io/utils/net"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

const (
	Ipv4Regex                    = `((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`
	ClusterBaseDomainPlaceholder = "<CLUSTER_BASE_DOMAIN>"
)

// Anonymizer is used to anonymize sensitive data.
// Config can be used to disable anonymization of particular types of data.
type Anonymizer struct {
	configObserver    *configobserver.Controller
	clusterBaseDomain string
	networks          []*net.IPNet
	ipRegex           *regexp.Regexp
}

// NewAnonymizer creates a new instance of anonymizer with a provided config observer and sensitive data
func NewAnonymizer(
	configObserver *configobserver.Controller, clusterBaseDomain string, networks []string,
) (*Anonymizer, error) {
	networks = append(networks, "127.0.0.1/8")

	cidrs, err := k8snet.ParseCIDRs(networks)
	if err != nil {
		return nil, err
	}

	return &Anonymizer{
		configObserver:    configObserver,
		clusterBaseDomain: strings.TrimSpace(clusterBaseDomain),
		networks:          cidrs,
		ipRegex:           regexp.MustCompile(Ipv4Regex),
	}, nil
}

// NewAnonymizer creates a new instance of anonymizer with a provided config observer and openshift config client
func NewAnonymizerFromConfigClient(
	ctx context.Context, configObserver *configobserver.Controller, configClient configv1client.ConfigV1Interface,
) (*Anonymizer, error) {
	baseDomain, err := utils.GetClusterBaseDomain(ctx, configClient)
	if err != nil {
		return nil, err
	}

	networksConfig, err := configClient.Networks().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var networks []string
	for _, network := range networksConfig.Spec.ClusterNetwork {
		networks = append(networks, network.CIDR)
	}
	for _, network := range networksConfig.Spec.ServiceNetwork {
		networks = append(networks, network)
	}
	for _, network := range networksConfig.Spec.ExternalIP.AutoAssignCIDRs {
		networks = append(networks, network)
	}
	for _, network := range networksConfig.Spec.ExternalIP.Policy.AllowedCIDRs {
		networks = append(networks, network)
	}
	for _, network := range networksConfig.Spec.ExternalIP.Policy.RejectedCIDRs {
		networks = append(networks, network)
	}

	return NewAnonymizer(configObserver, baseDomain, networks)
}

// AnonymizeMemoryRecord takes record.MemoryRecord, removes the sensitive data from it and returns the same object
func (anonymizer *Anonymizer) AnonymizeMemoryRecord(memoryRecord *record.MemoryRecord) *record.MemoryRecord {
	if anonymizer.configObserver == nil {
		return memoryRecord
	}

	if !anonymizer.configObserver.Config().EnableGlobalObfuscation {
		return memoryRecord
	}

	if len(anonymizer.clusterBaseDomain) != 0 {
		memoryRecord.Data = bytes.ReplaceAll(
			memoryRecord.Data,
			[]byte(anonymizer.clusterBaseDomain),
			[]byte(ClusterBaseDomainPlaceholder),
		)
		memoryRecord.Name = strings.ReplaceAll(
			memoryRecord.Name,
			anonymizer.clusterBaseDomain,
			ClusterBaseDomainPlaceholder,
		)
	}

	// We could use something like https://github.com/yl2chen/cidranger but we shouldn't typically have many networks
	// so it's fine to just iterate over them

	memoryRecord.Data = anonymizer.ipRegex.ReplaceAllFunc(memoryRecord.Data, func(originalIPBytes []byte) []byte {
		originalIP := net.ParseIP(string(originalIPBytes))
		if originalIP == nil {
			klog.Warningf("Unable to parse IP '%v'", string(originalIPBytes))
			// Unable to parse an IP, so just return whatever it is. It shouldn't happen.
			return originalIPBytes
		}

		isIPv4 := originalIP.To4() != nil

		if !isIPv4 {
			// TODO: to be implemented later
			// the problem is that some strings can be incorrectly identified as ip v6
			// we can try looking only for those which are wrapped by quotes or something
			return originalIPBytes
		}

		for _, network := range anonymizer.networks {
			if network.Contains(originalIP) {
				// return the first matched network
				return []byte(network.IP.String())
			}
		}

		if originalIP.To4() != nil {
			// ipv4
			return []byte("0.0.0.0")
		}
		// ipv6
		return []byte("::")
	})

	return memoryRecord
}
