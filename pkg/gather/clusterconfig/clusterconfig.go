package clusterconfig

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/client-go/config/clientset/versioned/scheme"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"

	"encoding/base64"
	"encoding/pem"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/openshift/insights-operator/pkg/record"
)

var (
	serializer     = scheme.Codecs.LegacyCodec(configv1.SchemeGroupVersion)
	kubeSerializer = kubescheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)

	// maxEventTimeInterval represents the "only keep events that are maximum 1h old"
	// TODO: make this dynamic like the reporting window based on configured interval
	maxEventTimeInterval = 1 * time.Hour
)

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	client        configv1client.ConfigV1Interface
	coreClient    corev1client.CoreV1Interface
	metricsClient rest.Interface
	certClient    certificatesv1beta1.CertificatesV1beta1Interface
	lock          sync.Mutex
	lastVersion   *configv1.ClusterVersion
}

// New creates new Gatherer
func New(client configv1client.ConfigV1Interface, coreClient corev1client.CoreV1Interface, certClient certificatesv1beta1.CertificatesV1beta1Interface, metricsClient rest.Interface) *Gatherer {
	return &Gatherer{
		client:        client,
		coreClient:    coreClient,
		certClient:    certClient,
		metricsClient: metricsClient,
	}
}

var reInvalidUIDCharacter = regexp.MustCompile(`[^a-z0-9\-]`)

// Gather is hosting and calling all the recording functions
func (i *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	return record.Collect(ctx, recorder,
		func() ([]record.Record, []error) {
			if i.metricsClient == nil {
				return nil, nil
			}
			data, err := i.metricsClient.Get().AbsPath("federate").
				Param("match[]", "ALERTS").
				Param("match[]", "etcd_object_counts").
				Param("match[]", "cluster_installer").
				DoRaw()
			if err != nil {
				// write metrics errors to the file format as a comment
				klog.Errorf("Unable to retrieve most recent metrics: %v", err)
				return []record.Record{{Name: "config/metrics", Item: RawByte(fmt.Sprintf("# error: %v\n", err))}}, nil
			}
			return []record.Record{
				{Name: "config/metrics", Item: RawByte(data)},
			}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.ClusterOperators().List(metav1.ListOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			records := make([]record.Record, 0, len(config.Items))
			for i := range config.Items {
				records = append(records, record.Record{Name: fmt.Sprintf("config/clusteroperator/%s", config.Items[i].Name), Item: ClusterOperatorAnonymizer{&config.Items[i]}})
			}
			namespaceEventsCollected := sets.NewString()

			now := time.Now()
			for _, item := range config.Items {
				if isHealthyOperator(&item) {
					continue
				}
				for _, namespace := range namespacesForOperator(&item) {
					pods, err := i.coreClient.Pods(namespace).List(metav1.ListOptions{})
					if err != nil {
						klog.V(2).Infof("Unable to find pods in namespace %s for failing operator %s", namespace, item.Name)
						continue
					}
					for i := range pods.Items {
						if isHealthyPod(&pods.Items[i], now) {
							continue
						}
						records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/%s", pods.Items[i].Namespace, pods.Items[i].Name), Item: PodAnonymizer{&pods.Items[i]}})
					}
					if namespaceEventsCollected.Has(namespace) {
						continue
					}
					namespaceRecords, errs := i.gatherNamespaceEvents(namespace)
					if len(errs) > 0 {
						klog.V(2).Infof("Unable to collect events for namespace %q: %#v", namespace, errs)
						continue
					}
					records = append(records, namespaceRecords...)
					namespaceEventsCollected.Insert(namespace)
				}
			}
			return records, nil
		},
		func() ([]record.Record, []error) {
			nodes, err := i.coreClient.Nodes().List(metav1.ListOptions{})
			if err != nil {
				return nil, []error{err}
			}
			records := make([]record.Record, 0, len(nodes.Items))
			for i := range nodes.Items {
				if isHealthyNode(&nodes.Items[i]) {
					continue
				}
				records = append(records, record.Record{Name: fmt.Sprintf("config/node/%s", nodes.Items[i].Name), Item: NodeAnonymizer{&nodes.Items[i]}})
			}

			return records, nil
		},
		func() ([]record.Record, []error) {
			cms, err := i.coreClient.ConfigMaps("openshift-config").List(metav1.ListOptions{})
			if err != nil {
				return nil, []error{err}
			}
			records := make([]record.Record, 0, len(cms.Items))
			for i := range cms.Items {
				for dk, dv := range cms.Items[i].Data {
					records = append(records, record.Record{Name: fmt.Sprintf("config/configmaps/%s/%s", cms.Items[i].Name, dk), Item: ConfigMapAnonymizer{v: []byte(dv), encodeBase64: false}})
				}
				for dk, dv := range cms.Items[i].BinaryData {
					records = append(records, record.Record{Name: fmt.Sprintf("config/configmaps/%s/%s", cms.Items[i].Name, dk), Item: ConfigMapAnonymizer{v: dv, encodeBase64: true}})
				}
			}

			return records, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.ClusterVersions().Get("version", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			i.setClusterVersion(config)
			return []record.Record{{Name: "config/version", Item: ClusterVersionAnonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			version := i.ClusterVersion()
			if version == nil {
				return nil, nil
			}
			return []record.Record{{Name: "config/id", Item: Raw{string(version.Spec.ClusterID)}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.Infrastructures().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/infrastructure", Item: InfrastructureAnonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.Networks().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/network", Item: Anonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.Authentications().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/authentication", Item: Anonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.FeatureGates().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/featuregate", Item: FeatureGateAnonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.OAuths().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/oauth", Item: Anonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.Ingresses().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/ingress", Item: IngressAnonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			config, err := i.client.Proxies().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			return []record.Record{{Name: "config/proxy", Item: ProxyAnonymizer{config}}}, nil
		},
		func() ([]record.Record, []error) {
			requests, err := i.certClient.CertificateSigningRequests().List(metav1.ListOptions{})
			if errors.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, []error{err}
			}
			csrs, err := FromCSRs(requests).Anonymize().Filter(IncludeCSR).Select()
			if err != nil {
				return nil, []error{err}
			}
			records := make([]record.Record, len(csrs))
			for i, sr := range csrs {
				records[i] = record.Record{Name: fmt.Sprintf("config/certificatesigningrequests/%s", sr.ObjectMeta.Name), Item: sr}
			}
			return records, nil
		},
	)
}

func (i *Gatherer) gatherNamespaceEvents(namespace string) ([]record.Record, []error) {
	// do not accidentally collect events for non-openshift namespace
	if !strings.HasPrefix(namespace, "openshift-") {
		return []record.Record{}, nil
	}
	events, err := i.coreClient.Events(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	// filter the event list to only recent events
	oldestEventTime := time.Now().Add(-maxEventTimeInterval)
	var filteredEventIndex []int
	for i := range events.Items {
		if events.Items[i].LastTimestamp.Time.Before(oldestEventTime) {
			continue
		}
		filteredEventIndex = append(filteredEventIndex, i)

	}
	compactedEvents := CompactedEventList{Items: make([]CompactedEvent, len(filteredEventIndex))}
	for i, index := range filteredEventIndex {
		compactedEvents.Items[i] = CompactedEvent{
			Namespace:     events.Items[index].Namespace,
			LastTimestamp: events.Items[index].LastTimestamp.Time,
			Reason:        events.Items[index].Reason,
			Message:       events.Items[index].Message,
		}
	}
	sort.Slice(compactedEvents.Items, func(i, j int) bool {
		return compactedEvents.Items[i].LastTimestamp.Before(compactedEvents.Items[j].LastTimestamp)
	})
	return []record.Record{{Name: fmt.Sprintf("events/%s", namespace), Item: EventAnonymizer{&compactedEvents}}}, nil
}

// RawByte is skipping Marshalling from byte slice
type RawByte []byte

// Marshal just returns bytes
func (r RawByte) Marshal(_ context.Context) ([]byte, error) {
	return r, nil
}

// GetExtension returns extension for "id" file - none
func (r RawByte) GetExtension() string {
	return ""
}

// Raw is another simplification of marshalling from string
type Raw struct{ string }

// Marshal returns raw bytes
func (r Raw) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.string), nil
}

// GetExtension returns extension for raw marshaller
func (r Raw) GetExtension() string {
	return ""
}

// Anonymizer returns serialized runtime.Object without change
type Anonymizer struct{ runtime.Object }

// Marshal serializes with OpenShift client-go serializer
func (a Anonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.Object)
}

// GetExtension returns extension for anonymized openshift objects
func (a Anonymizer) GetExtension() string {
	return "json"
}

// InfrastructureAnonymizer anonymizes infrastructure
type InfrastructureAnonymizer struct{ *configv1.Infrastructure }

// Marshal serializes Infrastructure with anonymization
func (a InfrastructureAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, anonymizeInfrastructure(a.Infrastructure))
}

// GetExtension returns extension for anonymized infra objects
func (a InfrastructureAnonymizer) GetExtension() string {
	return "json"
}

func anonymizeInfrastructure(config *configv1.Infrastructure) *configv1.Infrastructure {
	config.Status.APIServerURL = anonymizeURL(config.Status.APIServerURL)
	config.Status.EtcdDiscoveryDomain = anonymizeURL(config.Status.EtcdDiscoveryDomain)
	config.Status.InfrastructureName = anonymizeURL(config.Status.InfrastructureName)
	config.Status.APIServerInternalURL = anonymizeURL(config.Status.APIServerInternalURL)
	return config
}

// ClusterVersionAnonymizer is serializing ClusterVersion with anonymization
type ClusterVersionAnonymizer struct{ *configv1.ClusterVersion }

// Marshal serializes ClusterVersion with anonymization
func (a ClusterVersionAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.ClusterVersion.Spec.Upstream = configv1.URL(anonymizeURL(string(a.ClusterVersion.Spec.Upstream)))
	return runtime.Encode(serializer, a.ClusterVersion)
}

// GetExtension returns extension for anonymized cluster version objects
func (a ClusterVersionAnonymizer) GetExtension() string {
	return "json"
}

// FeatureGateAnonymizer implements serializaton of FeatureGate with anonymization
type FeatureGateAnonymizer struct{ *configv1.FeatureGate }

// Marshal serializes FeatureGate with anonymization
func (a FeatureGateAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.FeatureGate)
}

// GetExtension returns extension for anonymized cluster version objects
func (a FeatureGateAnonymizer) GetExtension() string {
	return "json"
}

// IngressAnonymizer implements serialization with marshalling
type IngressAnonymizer struct{ *configv1.Ingress }

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a IngressAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Ingress.Spec.Domain = anonymizeURL(a.Ingress.Spec.Domain)
	return runtime.Encode(serializer, a.Ingress)
}

// GetExtension returns extension for anonymized ingress objects
func (a IngressAnonymizer) GetExtension() string {
	return "json"
}

// CompactedEvent holds one Namespace Event
type CompactedEvent struct {
	Namespace     string    `json:"namespace"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message"`
}

// CompactedEventList is collection of events
type CompactedEventList struct {
	Items []CompactedEvent `json:"items"`
}

// EventAnonymizer implements serializaion of Events with anonymization
type EventAnonymizer struct{ *CompactedEventList }

// Marshal serializes Events with anonymization
func (a EventAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(a.CompactedEventList)
}

// GetExtension returns extension for anonymized event objects
func (a EventAnonymizer) GetExtension() string {
	return "json"
}

// ProxyAnonymizer implements serialization of HttpProxy/NoProxy with anonymization
type ProxyAnonymizer struct{ *configv1.Proxy }

// Marshal implements Proxy serialization with anonymization
func (a ProxyAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Proxy.Spec.HTTPProxy = anonymizeURLCSV(a.Proxy.Spec.HTTPProxy)
	a.Proxy.Spec.HTTPSProxy = anonymizeURLCSV(a.Proxy.Spec.HTTPSProxy)
	a.Proxy.Spec.NoProxy = anonymizeURLCSV(a.Proxy.Spec.NoProxy)
	a.Proxy.Spec.ReadinessEndpoints = anonymizeURLSlice(a.Proxy.Spec.ReadinessEndpoints)
	a.Proxy.Status.HTTPProxy = anonymizeURLCSV(a.Proxy.Status.HTTPProxy)
	a.Proxy.Status.HTTPSProxy = anonymizeURLCSV(a.Proxy.Status.HTTPSProxy)
	a.Proxy.Status.NoProxy = anonymizeURLCSV(a.Proxy.Status.NoProxy)
	return runtime.Encode(serializer, a.Proxy)
}

// GetExtension returns extension for anonymized proxy objects
func (a ProxyAnonymizer) GetExtension() string {
	return "json"
}

func anonymizeURLCSV(s string) string {
	strs := strings.Split(s, ",")
	outSlice := anonymizeURLSlice(strs)
	return strings.Join(outSlice, ",")
}

func anonymizeURLSlice(in []string) []string {
	outSlice := []string{}
	for _, str := range in {
		outSlice = append(outSlice, anonymizeURL(str))
	}
	return outSlice
}

var reURL = regexp.MustCompile(`[^\.\-/\:]`)

func anonymizeURL(s string) string { return reURL.ReplaceAllString(s, "x") }

// ClusterOperatorAnonymizer implements serialization of ClusterOperator without change
type ClusterOperatorAnonymizer struct{ *configv1.ClusterOperator }

// Marshal serializes ClusterOperator
func (a ClusterOperatorAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.ClusterOperator)
}

// GetExtension returns extension for anonymized cluster operator objects
func (a ClusterOperatorAnonymizer) GetExtension() string {
	return "json"
}

func isHealthyOperator(operator *configv1.ClusterOperator) bool {
	for _, condition := range operator.Status.Conditions {
		switch {
		case condition.Type == configv1.OperatorDegraded && condition.Status == configv1.ConditionTrue,
			condition.Type == configv1.OperatorAvailable && condition.Status == configv1.ConditionFalse:
			return false
		}
	}
	return true
}

func namespacesForOperator(operator *configv1.ClusterOperator) []string {
	var ns []string
	for _, ref := range operator.Status.RelatedObjects {
		if ref.Resource == "namespaces" {
			ns = append(ns, ref.Name)
		}
	}
	return ns
}

// NodeAnonymizer implements serialization of Node with anonymization
type NodeAnonymizer struct{ *corev1.Node }

// Marshal implements serialization of Node with anonymization
func (a NodeAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, anonymizeNode(a.Node))
}

// GetExtension returns extension for anonymized node objects
func (a NodeAnonymizer) GetExtension() string {
	return "json"
}

func anonymizeNode(node *corev1.Node) *corev1.Node {
	for k := range node.Annotations {
		if isProductNamespacedKey(k) {
			continue
		}
		node.Annotations[k] = ""
	}
	for k, v := range node.Labels {
		if isProductNamespacedKey(k) {
			continue
		}
		node.Labels[k] = anonymizeString(v)
	}
	for i := range node.Status.Addresses {
		node.Status.Addresses[i].Address = anonymizeURL(node.Status.Addresses[i].Address)
	}
	node.Status.NodeInfo.BootID = anonymizeString(node.Status.NodeInfo.BootID)
	node.Status.NodeInfo.SystemUUID = anonymizeString(node.Status.NodeInfo.SystemUUID)
	node.Status.NodeInfo.MachineID = anonymizeString(node.Status.NodeInfo.MachineID)
	node.Status.Images = nil
	return node
}

func anonymizeString(s string) string {
	return strings.Repeat("x", len(s))
}

func isProductNamespacedKey(key string) bool {
	return strings.Contains(key, "openshift.io/") || strings.Contains(key, "k8s.io/") || strings.Contains(key, "kubernetes.io/")
}

func isHealthyNode(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

// PodAnonymizer implements serialization with anonymization for a Pod
type PodAnonymizer struct{ *corev1.Pod }

// Marshal implements serialization of a Pod with anonymization
func (a PodAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, anonymizePod(a.Pod))
}

// GetExtension returns extension for anonymized pod objects
func (a PodAnonymizer) GetExtension() string {
	return "json"
}

func anonymizePod(pod *corev1.Pod) *corev1.Pod {
	// pods gathered from openshift namespaces and cluster operators are expected to be under our control and contain
	// no sensitive information
	return pod
}

func isHealthyPod(pod *corev1.Pod, now time.Time) bool {
	// pending pods may be unable to schedule or start due to failures, and the info they provide in status is important
	// for identifying why scheduling has not happened
	if pod.Status.Phase == corev1.PodPending {
		if now.Sub(pod.CreationTimestamp.Time) > 2*time.Minute {
			return false
		}
	}
	// pods that have containers that have terminated with non-zero exit codes are considered failure
	for _, status := range pod.Status.InitContainerStatuses {
		if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
			return false
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			return false
		}
		if status.RestartCount > 0 {
			return false
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
			return false
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			return false
		}
		if status.RestartCount > 0 {
			return false
		}
	}
	return true
}

func (i *Gatherer) setClusterVersion(version *configv1.ClusterVersion) {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.lastVersion != nil && i.lastVersion.ResourceVersion == version.ResourceVersion {
		return
	}
	i.lastVersion = version.DeepCopy()
}

// ClusterVersion returns Version for this cluster, which is set by running version during Gathering
func (i *Gatherer) ClusterVersion() *configv1.ClusterVersion {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.lastVersion
}

// ConfigMapAnonymizer implements serialization of configmap
// and potentially anonymizes if it is a certificate
type ConfigMapAnonymizer struct {
	v            []byte
	encodeBase64 bool
}

// Marshal implements serialization of Node with anonymization
func (a ConfigMapAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	c := []byte(anonymizeConfigMap(a.v))
	if a.encodeBase64 {
		buff := make([]byte, base64.StdEncoding.EncodedLen(len(c)))
		base64.StdEncoding.Encode(buff, []byte(c))
		c = buff
	}
	return c, nil
}

// GetExtension returns extension for anonymized configmap objects
func (a ConfigMapAnonymizer) GetExtension() string {
	return ""
}

func anonymizeConfigMap(dv []byte) string {
	anonymizedPemBlock := `-----BEGIN CERTIFICATE-----
ANONYMIZED
-----END CERTIFICATE-----
`
	var sb strings.Builder
	r := dv
	for {
		var block *pem.Block
		block, r = pem.Decode(r)
		if block == nil {
			// cannot be extracted
			return string(dv)
		}
		sb.WriteString(anonymizedPemBlock)
		if len(r) == 0 {
			break
		}
	}
	return sb.String()
}
