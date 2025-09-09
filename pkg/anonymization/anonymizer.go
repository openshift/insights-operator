// Package anonymization provides Anonymizer which is used to anonymize sensitive data. At the moment,
// anonymization is applied to all the data before storing it in the archive(see AnonymizeMemoryRecordFunction).
// If you want to enable the anonymization you need to set "enableGlobalObfuscation" to "true" in config
// or "support" secret in "openshift-config" namespace, the anonymizer object then will be created and used
// (see pkg/controller/operator.go and pkg/controller/gather_job.go).
// When enabled, the following data will be anonymized:
//   - cluster base domain. For example, if the cluster base domain is `openshift.example.com`,
//     all the occurrences of this keyword will be replaced with `<CLUSTER_BASE_DOMAIN>`,
//     `cluster-api.openshift.example.com` will become `cluster-api.<CLUSTER_BASE_DOMAIN>`
//   - IPv4 addresses. Using a config client, it retrieves cluster networks and uses them to anonymize IP addresses
//     preserving subnet information. For example, if you have the following networks in your cluster:
//     "10.128.0.0/14", "172.30.0.0/16", "127.0.0.0/8"(added by default) the anonymization will handle the IPs like this:
//   - 10.128.0.0 -> 10.128.0.0  // subnetwork itself won't be anonymized
//   - 10.128.0.55 -> 10.128.0.1
//   - 10.128.0.56 -> 10.128.0.2
//   - 10.128.0.55 -> 10.128.0.1
//     // anonymizer maintains a translation table to replace the same original IPs with the same obfuscated IPs
//   - 10.129.0.0 -> 10.128.0.3
//   - 172.30.0.5 -> 172.30.0.1  // new subnet, so we use a new set of fake IPs
//   - 127.0.0.1 -> 127.0.0.1  // it was the first IP, so the new IP matched the original in this case
//   - 10.0.134.130 -> 0.0.0.0  // ip doesn't match any subnet, we replace such IPs with 0.0.0.0
package anonymization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/insights/v1alpha2"
	networkv1 "github.com/openshift/api/network/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	k8snet "k8s.io/utils/net"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

// norevive
const (
	Ipv4Regex                            = `((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`
	Ipv4NetworkRegex                     = Ipv4Regex + "/([0-9]{1,2})"
	Ipv4AddressOrNetworkRegex            = Ipv4Regex + "(/([0-9]{1,2}))?"
	ClusterBaseDomainPlaceholder         = "<CLUSTER_BASE_DOMAIN>"
	ClusterHostPlaceholder               = "<CLUSTER_DOMAIN_HOST>"
	UnableToCreateAnonymizerErrorMessage = "Unable to create anonymizer, " +
		"some data won't be anonymized(ipv4 and cluster base domain). The error is %v"
	clusterNetworksRecordName = "config/network.json"
	clusterConfigV1RecordName = "config/configmaps/kube-system/cluster-config-v1/install-config"
	hostSubnetRecordPrefix    = "config/hostsubnet/"
)

var (
	// TranslationTableSecretName defines the secret name to store the translation table
	TranslationTableSecretName = "obfuscation-translation-table" //nolint: gosec
	secretAPIVersion           = "v1"
	secretKind                 = "Secret"
	secretNamespace            = "openshift-insights"
	once                       sync.Once
)

type subnetInformation struct {
	network net.IPNet
	lastIP  net.IP
}

type DataAnonymizer interface {
	// AnonymizeData should be used to run anonymization
	AnonymizeData() error
	// IsEnabled should be used to check if a given anonymization is enabled
	IsEnabled() bool
}

// Anonymizer is used to anonymize sensitive data.
// Config can be used to enable anonymization of cluster base domain
// and obfuscation of IPv4 addresses
type Anonymizer struct {
	sensitiveValues  map[string]string
	networks         []subnetInformation
	translationTable map[string]string
	ipNetworkRegex   *regexp.Regexp
	secretsClient    corev1client.SecretInterface
	configurator     configobserver.Interface
	dataPolicy       v1alpha2.DataPolicyOption
	configClient     configv1client.ConfigV1Interface
	networkClient    networkv1client.NetworkV1Interface
	gatherKubeClient kubernetes.Interface
	runningInCluster bool
	anonymizers      []DataAnonymizer
}

type ConfigProvider interface {
	Config() *config.Controller
}

// NewAnonymizerFromConfigClient creates a new instance of anonymizer with a provided openshift config client
func NewAnonymizerFromConfigClient(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	gatherKubeClient kubernetes.Interface,
	configClient configv1client.ConfigV1Interface,
	networkClient networkv1client.NetworkV1Interface,
	configurator configobserver.Interface,
	dataPolicies []v1alpha2.DataPolicyOption,
	sensitiveVals map[string]string,
) (*Anonymizer, error) {
	anonBuilder := &AnonBuilder{}
	anonBuilder.
		WithConfigClient(configClient).
		WithConfigurator(configurator).
		WithDataPolicies(dataPolicies...).
		WithKubeClient(gatherKubeClient).
		WithNetworkClient(networkClient).
		WithRunningInCluster(true).
		WithSecretsClient(kubeClient.CoreV1().Secrets(secretNamespace))

	for value, placeholder := range sensitiveVals {
		anonBuilder.WithSensitiveValue(value, placeholder)
	}

	baseDomain, err := utils.GetClusterBaseDomain(ctx, configClient)
	if err != nil {
		return nil, err
	}
	anonBuilder.WithSensitiveValue(baseDomain, ClusterBaseDomainPlaceholder)

	return anonBuilder.Build()
}

func (anonymizer *Anonymizer) readNetworkConfigs() error {
	if !anonymizer.runningInCluster {
		return nil
	}
	networksConfig, err := anonymizer.configClient.Networks().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}

	clusterConfigV1, err := anonymizer.gatherKubeClient.CoreV1().
		ConfigMaps("kube-system").Get(context.Background(), "cluster-config-v1", metav1.GetOptions{})
	if err != nil {
		return err
	}

	var networks []string

	// hostsubnets are needed for egress subnets (on SDN clusters only)
	hostSubnets, err := anonymizer.networkClient.HostSubnets().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Infof("unable to find HostSubnets, could be OVN cluster: %v", err)
		networks = getNetworksForAnonymizer(networksConfig, clusterConfigV1, nil)
	} else {
		networks = getNetworksForAnonymizer(networksConfig, clusterConfigV1, hostSubnets.Items)
	}
	cidrs, err := k8snet.ParseCIDRs(networks)
	if err != nil {
		return err
	}

	for _, network := range cidrs {
		lastIP := network.IP
		anonymizer.networks = append(anonymizer.networks, subnetInformation{
			network: *network,
			lastIP:  lastIP,
		})
	}
	return nil
}

func GetNetworksForAnonymizerFromRecords(records map[string]*record.MemoryRecord) ([]string, error) {
	clusterNetworksRecord, found := records[clusterNetworksRecordName]
	if !found {
		return nil, fmt.Errorf(
			"unable to find the record containing cluster networks required for anonymizer to work correctly",
		)
	}

	var clusterNetworks configv1.Network
	err := json.Unmarshal(clusterNetworksRecord.Data, &clusterNetworks)
	if err != nil {
		return nil, err
	}

	var clusterConfigV1 corev1.ConfigMap

	clusterConfigV1Record, found := records[clusterConfigV1RecordName]
	if !found {
		klog.Warningf("record %v was not found, some networks won't be obfuscated", clusterConfigV1RecordName)
	} else {
		clusterConfigV1.Data = make(map[string]string, 1)
		clusterConfigV1.Data["install-config"] = string(clusterConfigV1Record.Data)
	}

	// for egress cidrs
	var hostSubnets []networkv1.HostSubnet

	for name, rec := range records {
		if !strings.HasPrefix(name, hostSubnetRecordPrefix) {
			continue
		}

		var hostSubnet networkv1.HostSubnet

		err := json.Unmarshal(rec.Data, &hostSubnet)
		if err != nil {
			return nil, err
		}

		hostSubnets = append(hostSubnets, hostSubnet)
	}

	if len(hostSubnets) == 0 {
		klog.Warningf("no record with prefix %v was found, egress IPs won't be obfuscated", hostSubnetRecordPrefix)
	}

	return getNetworksForAnonymizer(&clusterNetworks, &clusterConfigV1, hostSubnets), nil
}

func getNetworksForAnonymizer(
	clusterNetworks *configv1.Network, clusterConfigV1 *corev1.ConfigMap, hostSubnets []networkv1.HostSubnet,
) []string {
	networks := append(
		[]string{"127.0.0.0/8"},
		getNetworksFromClusterNetworksConfig(clusterNetworks)...,
	)

	// we just ignore it in case it doesn't exist
	if clusterConfigV1 != nil {
		installConfig, found := clusterConfigV1.Data["install-config"]
		if found {
			networkRegex := regexp.MustCompile(Ipv4NetworkRegex)
			networks = append(networks, networkRegex.FindAllString(installConfig, -1)...)
		}
	}

	// egress subnets
	for i := range hostSubnets {
		hostSubnet := &hostSubnets[i]
		for _, egressCIDR := range hostSubnet.EgressCIDRs {
			networks = append(networks, string(egressCIDR))
		}
	}

	// ovn clusters don't have hostsubnet objects and their egress CIDR is 192.168.126.0/18
	// nolint:lll
	// https://docs.openshift.com/container-platform/4.8/networking/ovn_kubernetes_network_provider/configuring-egress-ips-ovn.html#configuring-egress-ips-ovn
	if len(hostSubnets) == 0 {
		networks = append(networks, "192.168.126.0/18")
	}

	sortNetworks(networks)

	return networks
}

func getNetworksFromClusterNetworksConfig(networksConfig *configv1.Network) []string {
	var networks []string

	for _, network := range networksConfig.Spec.ClusterNetwork {
		networks = append(networks, network.CIDR)
	}
	networks = append(networks, networksConfig.Spec.ServiceNetwork...)

	if networksConfig.Spec.ExternalIP == nil {
		return networks
	}

	networks = append(networks, networksConfig.Spec.ExternalIP.AutoAssignCIDRs...)
	networks = append(networks, networksConfig.Spec.ExternalIP.Policy.AllowedCIDRs...)
	networks = append(networks, networksConfig.Spec.ExternalIP.Policy.RejectedCIDRs...)

	return networks
}

func sortNetworks(networks []string) {
	// we're sorting by subnet lengths, if they are the same, we use subnet itself
	utils.SortAndRemoveDuplicates(&networks, func(i, j int) bool {
		if !strings.Contains(networks[i], "/") || !strings.Contains(networks[j], "/") {
			return networks[i] > networks[j]
		}

		network1 := strings.Split(networks[i], "/")
		network2 := strings.Split(networks[j], "/")

		// first we compare by subnet lengths, but if they are equal, we compare the subnet itself
		if network1[1] != network2[1] {
			return network1[1] > network2[1]
		}

		return network1[0] > network2[0]
	})
}

// NewAnonymizerFromConfig creates a new instance of anonymizer with a provided kubeconfig
func NewAnonymizerFromConfig(
	ctx context.Context,
	gatherKubeConfig *rest.Config,
	gatherProtoKubeConfig *rest.Config,
	protoKubeConfig *rest.Config,
	configurator configobserver.Interface,
	dataPolicy []v1alpha2.DataPolicyOption,
) (*Anonymizer, error) {
	sensitiveVals := make(map[string]string)
	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return nil, err
	}

	gatherKubeClient, err := kubernetes.NewForConfig(gatherProtoKubeConfig)
	if err != nil {
		return nil, err
	}

	configClient, err := configv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return nil, err
	}

	networkClient, err := networkv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return nil, err
	}

	infrastructure, err := configClient.Infrastructures().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	clientHosts := []string{
		protoKubeConfig.Host,
		gatherProtoKubeConfig.Host,
		gatherKubeConfig.Host,
	}
	if err := addAPIDomainsForAnonymization(clientHosts, infrastructure, sensitiveVals); err != nil {
		return nil, fmt.Errorf("failed to add API domains for anonymization: %w", err)
	}

	return NewAnonymizerFromConfigClient(ctx,
		kubeClient, gatherKubeClient, configClient, networkClient,
		configurator, dataPolicy, sensitiveVals,
	)
}

// AnonymizeMemoryRecord takes record.MemoryRecord, removes the sensitive data from it and returns the same object
func (anonymizer *Anonymizer) AnonymizeMemoryRecord(memoryRecord *record.MemoryRecord) *record.MemoryRecord {
	// lazy init of network information
	once.Do(func() {
		err := anonymizer.readNetworkConfigs()
		if err != nil {
			klog.Errorf("failed to read network config: %v", err)
		}
	})

	for value, placeholder := range anonymizer.sensitiveValues {
		memoryRecord.Data = bytes.ReplaceAll(
			memoryRecord.Data,
			[]byte(value),
			[]byte(placeholder),
		)
		memoryRecord.Name = strings.ReplaceAll(
			memoryRecord.Name,
			value,
			placeholder,
		)
	}

	memoryRecord.Data = anonymizer.ipNetworkRegex.ReplaceAllFunc(memoryRecord.Data, func(originalIPBytes []byte) []byte {
		return []byte(anonymizer.ObfuscateIP(string(originalIPBytes)))
	})

	return memoryRecord
}

// ObfuscateIP takes an IP as a string and returns obfuscated version. If it exists in the translation table,
// we just take it from there, if it doesn't, we create an obfuscated version of this IP
// and record it to the translation table
func (anonymizer *Anonymizer) ObfuscateIP(ipStr string) string {
	if strings.Contains(ipStr, "/") {
		// we do not touch subnets themselves
		return ipStr
	}

	if obfuscatedIP, exists := anonymizer.translationTable[ipStr]; exists {
		return obfuscatedIP
	}

	originalIP := net.ParseIP(ipStr)
	if originalIP == nil {
		klog.Warningf("Unable to parse IP '%v'", ipStr)
		// Unable to parse an IP, so just return whatever it is. It shouldn't happen.
		return ipStr
	}

	isIPv4 := originalIP.To4() != nil

	if !isIPv4 {
		return ipStr
	}

	// We could use something like https://github.com/yl2chen/cidranger, but we shouldn't typically have many networks,
	// so it's fine to just iterate over them
	for i := range anonymizer.networks {
		networkInfo := &anonymizer.networks[i]
		network := &networkInfo.network

		if network.IP.Equal(originalIP) {
			return originalIP.String()
		}

		if network.Contains(originalIP) {
			nextIP, overflow := getNextIP(networkInfo.lastIP, network.Mask)
			if overflow {
				// it's very unlikely to ever happen
				klog.Warningf(
					"Anonymizer couldn't find the next IP for %v with mask %v", networkInfo.lastIP, network.Mask,
				)
			}

			networkInfo.lastIP = nextIP
			anonymizer.translationTable[ipStr] = nextIP.String()
			return nextIP.String()
		}
	}

	if isIPv4 {
		// ipv4
		return "0.0.0.0"
	}
	// ipv6
	return "::"
}

// StoreTranslationTable stores the translation table in a Secret in the openshift-insights namespace.
// The actual data is stored in the StringData portion of the Secret.
func (anonymizer *Anonymizer) StoreTranslationTable() *corev1.Secret {
	if len(anonymizer.translationTable) == 0 {
		return nil
	}
	defer anonymizer.ResetTranslationTable()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := anonymizer.secretsClient.Delete(ctx, TranslationTableSecretName, metav1.DeleteOptions{})
	if err != nil {
		klog.V(4).Infof("Failed to delete translation table secret. err: %s", err)
	}

	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       secretKind,
			APIVersion: secretAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: TranslationTableSecretName,
		},
		StringData: anonymizer.translationTable,
	}

	createOptions := metav1.CreateOptions{
		FieldManager: "insights-operator",
	}

	result, err := anonymizer.secretsClient.Create(ctx, &secret, createOptions)
	if err != nil {
		klog.Errorf("Failed to create the translation table secret. err: %s", err)
		return nil
	}
	klog.V(3).Infof("Created/Updated %s secret in %s namespace", TranslationTableSecretName, secretNamespace)
	return result
}

// ResetTranslationTable resets the translation table, so that the translation table of multiple gathers won't mix together.
func (anonymizer *Anonymizer) ResetTranslationTable() {
	anonymizer.translationTable = make(map[string]string)
}

// obfuscateNetworking tells whether Networking/IP addresses should be "obfuscated" or not
func obfuscateNetworking(o config.Obfuscation) bool {
	for _, ov := range o {
		if ov == config.Networking {
			return true
		}
	}
	return false
}

// IsObfuscationEnabled returns true if obfuscation(hiding IP and domain names) is enabled and false otherwise
func (anonymizer *Anonymizer) IsObfuscationEnabled() bool {
	obfuscation := anonymizer.configurator.Config().DataReporting.Obfuscation
	// support secret still has precedence
	if obfuscateNetworking(obfuscation) {
		return true
	}

	if anonymizer.dataPolicy != "" {
		return anonymizer.dataPolicy == v1alpha2.DataPolicyOptionObfuscateNetworking
	}

	return false
}

// getNextIP returns the next IP address in the current subnetwork and the flag indicating if there was an overflow
func getNextIP(originalIP net.IP, mask net.IPMask) (net.IP, bool) {
	isIpv4 := originalIP.To4() != nil

	fixArraySize := func(ip net.IP) net.IP {
		if isIpv4 {
			return utils.TakeLastNItemsFromByteArray(ip, net.IPv4len)
		}
		return utils.TakeLastNItemsFromByteArray(ip, net.IPv6len)
	}

	// for ipv4 take last 4 bytes because IPv4  can be represented as IPv6 internally
	originalIP = fixArraySize(originalIP)

	intValue := big.NewInt(0)

	for byteIndex, byteValue := range originalIP {
		// TODO - introduce proper int -> uint conversion
		shiftTo := uint((len(originalIP) - byteIndex - 1) * 8) //nolint:gosec
		intValue = intValue.Or(
			intValue, big.NewInt(0).Lsh(big.NewInt(int64(byteValue)), shiftTo),
		)
	}

	intValue = intValue.Add(intValue, big.NewInt(1))

	resultIP := net.IP(intValue.Bytes())

	// adding one can overflow the value leading to an array of 5 or 17 elements
	// and there is an options where we don't have enough leading zeros
	resultIP = fixArraySize(resultIP)

	originalIPNetwork := originalIP.Mask(mask)
	resultIPNetwork := resultIP.Mask(mask)

	if !originalIPNetwork.Equal(resultIPNetwork) {
		// network differs, there was an overflow
		// we still want networks to be the same
		var invertedMask net.IPMask
		for _, maskByte := range mask {
			invertedMask = append(invertedMask, maskByte^255)
		}

		resultHostIP := resultIP.Mask(invertedMask)

		// combine original IP's network part and result IP's host part
		intValue := big.NewInt(0).SetBytes(originalIPNetwork)
		intValue = intValue.Or(intValue, big.NewInt(0).SetBytes(resultHostIP))

		return intValue.Bytes(), true
	}

	return resultIP, false
}

// addAPIDomainsForAnonymization registers API server domains for anonymization.
// Uses infrastructure.Status.APIServerURL if available, otherwise processes all clientHosts.
func addAPIDomainsForAnonymization(clientHosts []string, infrastructure *configv1.Infrastructure, domainMap map[string]string) error {
	if infrastructure != nil && infrastructure.Status.APIServerURL != "" {
		// add the API server URL to the map
		return addParsedDomainToMap(infrastructure.Status.APIServerURL, domainMap, ClusterHostPlaceholder)
	}

	for _, clientHost := range clientHosts {
		// add the client host to the map
		if err := addParsedDomainToMap(clientHost, domainMap, ClusterHostPlaceholder); err != nil {
			return err
		}
	}

	return nil
}

// addParsedDomainToMap extracts hostname from address and adds it to the anonymization map.
// If URL parsing fails or no hostname found, uses the entire address as-is.
func addParsedDomainToMap(address string, domainMap map[string]string, placeholder string) error {
	parsedURL, err := url.Parse(address)
	if err != nil {
		return err
	}

	if parsedURL == nil || parsedURL.Hostname() == "" {
		domainMap[address] = placeholder
		return nil
	}

	// add domain to the map
	domainMap[parsedURL.Hostname()] = placeholder

	return nil
}
