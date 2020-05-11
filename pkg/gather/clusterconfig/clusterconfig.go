package clusterconfig

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"
	registryv1 "github.com/openshift/api/imageregistry/v1"
	openshiftscheme "github.com/openshift/client-go/config/clientset/versioned/scheme"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

var (
	openshiftSerializer = openshiftscheme.Codecs.LegacyCodec(configv1.SchemeGroupVersion)
	kubeSerializer      = kubescheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)

	// maxEventTimeInterval represents the "only keep events that are maximum 1h old"
	// TODO: make this dynamic like the reporting window based on configured interval
	maxEventTimeInterval = 1 * time.Hour

	registrySerializer serializer.CodecFactory
	registryScheme     = runtime.NewScheme()
)

func init() {
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
	registrySerializer = serializer.NewCodecFactory(registryScheme)
}

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	client         configv1client.ConfigV1Interface
	coreClient     corev1client.CoreV1Interface
	metricsClient  rest.Interface
	certClient     certificatesv1beta1.CertificatesV1beta1Interface
	registryClient imageregistryv1.ImageregistryV1Interface
	lock           sync.Mutex
	lastVersion    *configv1.ClusterVersion
}

// New creates new Gatherer
func New(client configv1client.ConfigV1Interface, coreClient corev1client.CoreV1Interface, certClient certificatesv1beta1.CertificatesV1beta1Interface, metricsClient rest.Interface,
	registryClient imageregistryv1.ImageregistryV1Interface) *Gatherer {
	return &Gatherer{
		client:         client,
		coreClient:     coreClient,
		certClient:     certClient,
		metricsClient:  metricsClient,
		registryClient: registryClient,
	}
}

var reInvalidUIDCharacter = regexp.MustCompile(`[^a-z0-9\-]`)

// Gather is hosting and calling all the recording functions
func (i *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	return record.Collect(ctx, recorder,
		GatherMostRecentMetrics(i),
		GatherClusterOperators(i),
		GatherUnhealthyNodes(i),
		GatherConfigMaps(i),
		GatherClusterVersion(i),
		GatherClusterID(i),
		GatherClusterInfrastructure(i),
		GatherClusterNetwork(i),
		GatherClusterAuthentication(i),
		GatherClusterImageRegistry(i),
		GatherClusterFeatureGates(i),
		GatherClusterOAuth(i),
		GatherClusterIngress(i),
		GatherClusterProxy(i),
		GatherCertificateSigningRequests(i),
	)
}

// GatherMostRecentMetrics gathers cluster Federated Monitoring metrics.
//
// The GET REST query to URL /federate
// Gathered metrics:
//   ALERTS
//   etcd_object_counts
//   cluster_installer
//
// Location in archive: config/metrics/
// See: docs/insights-archive-sample/config/metrics
func GatherMostRecentMetrics(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
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
		records := []record.Record{
			{Name: "config/metrics", Item: RawByte(data)},
		}

		return records, nil
	}
}

// GatherClusterOperators collects all ClusterOperators.
// It finds unhealthy Pods for unhealthy operators
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusteroperator.go#L62
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusteroperatorlist-v1config-openshift-io
//
// Location of operators in archive: config/clusteroperator/
// See: docs/insights-archive-sample/config/clusteroperator
// Location of pods in archive: config/pod/
func GatherClusterOperators(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
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
	}
}

// GatherUnhealthyNodes collects all unhealthy Nodes.
//
// The node is unhealthy when:
// the operator.Status.Conditions.Condition Type
//   is OperatorDegrated and Status is True or
//      OperatorAvailable and Status is False
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/node.go#L78
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#nodelist-v1core
//
// Location in archive: config/node/
func GatherUnhealthyNodes(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
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
	}
}

// GatherConfigMaps fetches the ConfigMaps from namespace openshift-config.
//
// Anonymization: If the content of ConfigMap contains a parseable PEM structure (like certificate) it removes the inside of PEM blocks.
// For ConfigMap of type BinaryData it is encoded as standard base64.
// In the archive under configmaps we store name of ConfigMap and then each ConfigMap Key. For example config/configmaps/CONFIGMAPNAME/CONFIGMAPKEY1
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/configmap.go#L80
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#configmaplist-v1core
//
// Location in archive: config/configmaps/
// See: docs/insights-archive-sample/config/configmaps
func GatherConfigMaps(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
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
	}
}

// GatherClusterVersion fetches the ClusterVersion - the ClusterVersion with name version.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusterversion.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusterversion-v1config-openshift-io
//
// Location in archive: config/version/
// See: docs/insights-archive-sample/config/version
func GatherClusterVersion(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.ClusterVersions().Get("version", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		i.setClusterVersion(config)
		return []record.Record{{Name: "config/version", Item: ClusterVersionAnonymizer{config}}}, nil
	}
}

// GatherClusterID stores ClusterID from ClusterVersion version
// This method uses data already collected by Get ClusterVersion. In particular field .Spec.ClusterID
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusterversion.go#L50
// Response see https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go#L38
//
// Location in archive: config/id/
// See: docs/insights-archive-sample/config/id
func GatherClusterID(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		version := i.ClusterVersion()
		if version == nil {
			return nil, nil
		}
		return []record.Record{{Name: "config/id", Item: Raw{string(version.Spec.ClusterID)}}}, nil
	}
}

// GatherClusterInfrastructure fetches the cluster Infrastructure - the Infrastructure with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/infrastructure.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#infrastructure-v1-config-openshift-io
//
// Location in archive: config/infrastructure/
// See: docs/insights-archive-sample/config/infrastructure
func GatherClusterInfrastructure(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.Infrastructures().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/infrastructure", Item: InfrastructureAnonymizer{config}}}, nil
	}
}

// GatherClusterNetwork fetches the cluster Network - the Network with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/network.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#network-v1-config-openshift-io
//
// Location in archive: config/network/
// See: docs/insights-archive-sample/config/network
func GatherClusterNetwork(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.Networks().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/network", Item: Anonymizer{config}}}, nil
	}
}

// GatherClusterAuthentication fetches the cluster Authentication - the Authentication with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/authentication.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#authentication-v1operator-openshift-io
//
// Location in archive: config/authentication/
// See: docs/insights-archive-sample/config/authentication
func GatherClusterAuthentication(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.Authentications().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/authentication", Item: Anonymizer{config}}}, nil
	}
}

// GatherClusterImageRegistry fetches the cluster Image Registry configuration
//
// Location in archive: config/imageregistry/
func GatherClusterImageRegistry(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.registryClient.Configs().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/imageregistry", Item: ImageRegistryAnonymizer{config}}}, nil
	}
}

// GatherClusterFeatureGates fetches the cluster FeatureGate - the FeatureGate with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/featuregate.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#featuregate-v1-config-openshift-io
//
// Location in archive: config/featuregate/
// See: docs/insights-archive-sample/config/featuregate
func GatherClusterFeatureGates(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.FeatureGates().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/featuregate", Item: FeatureGateAnonymizer{config}}}, nil
	}
}

// GatherClusterOAuth fetches the cluster OAuth - the OAuth with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/oauth.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#oauth-v1-config-openshift-io
//
// Location in archive: config/oauth/
// See: docs/insights-archive-sample/config/oauth
func GatherClusterOAuth(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.OAuths().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/oauth", Item: Anonymizer{config}}}, nil
	}
}

// GatherClusterIngress fetches the cluster Ingress - the Ingress with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/ingress.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#ingress-v1-config-openshift-io
//
// Location in archive: config/ingress/
// See: docs/insights-archive-sample/config/ingress
func GatherClusterIngress(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.Ingresses().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/ingress", Item: IngressAnonymizer{config}}}, nil
	}
}

// GatherClusterProxy fetches the cluster Proxy - the Proxy with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/proxy.go#L30
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#proxy-v1-config-openshift-io
//
// Location in archive: config/proxy/
// See: docs/insights-archive-sample/config/proxy
func GatherClusterProxy(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.client.Proxies().Get("cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/proxy", Item: ProxyAnonymizer{config}}}, nil
	}
}

// GatherCertificateSigningRequests collects anonymized CertificateSigningRequests.
// Collects CSRs which werent Verified, or when Now < ValidBefore or Now > ValidAfter
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/certificates/v1beta1/certificatesigningrequest.go#L78
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#certificatesigningrequestlist-v1beta1certificates
//
// Location in archive: config/certificatesigningrequests/
func GatherCertificateSigningRequests(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
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
	}
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

// Raw is another simplification of marshalling from string
type Raw struct{ string }

// Marshal returns raw bytes
func (r Raw) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.string), nil
}

// Anonymizer returns serialized runtime.Object without change
type Anonymizer struct{ runtime.Object }

// Marshal serializes with OpenShift client-go openshiftSerializer
func (a Anonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, a.Object)
}

// InfrastructureAnonymizer anonymizes infrastructure
type InfrastructureAnonymizer struct{ *configv1.Infrastructure }

// Marshal serializes Infrastructure with anonymization
func (a InfrastructureAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, anonymizeInfrastructure(a.Infrastructure))
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
	return runtime.Encode(openshiftSerializer, a.ClusterVersion)
}

// FeatureGateAnonymizer implements serializaton of FeatureGate with anonymization
type FeatureGateAnonymizer struct{ *configv1.FeatureGate }

// Marshal serializes FeatureGate with anonymization
func (a FeatureGateAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, a.FeatureGate)
}

// IngressAnonymizer implements serialization with marshalling
type ImageRegistryAnonymizer struct {
	*registryv1.Config
}

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a ImageRegistryAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Spec.HTTPSecret = anonymizeString(a.Spec.HTTPSecret)
	if a.Spec.Storage.S3 != nil {
		a.Spec.Storage.S3.Bucket = anonymizeString(a.Spec.Storage.S3.Bucket)
		a.Spec.Storage.S3.KeyID = anonymizeString(a.Spec.Storage.S3.KeyID)
		a.Spec.Storage.S3.RegionEndpoint = anonymizeString(a.Spec.Storage.S3.RegionEndpoint)
		a.Spec.Storage.S3.Region = anonymizeString(a.Spec.Storage.S3.Region)
	}
	if a.Spec.Storage.Azure != nil {
		a.Spec.Storage.Azure.AccountName = anonymizeString(a.Spec.Storage.Azure.AccountName)
		a.Spec.Storage.Azure.Container = anonymizeString(a.Spec.Storage.Azure.Container)
	}
	if a.Spec.Storage.GCS != nil {
		a.Spec.Storage.GCS.Bucket = anonymizeString(a.Spec.Storage.GCS.Bucket)
		a.Spec.Storage.GCS.ProjectID = anonymizeString(a.Spec.Storage.GCS.ProjectID)
		a.Spec.Storage.GCS.KeyID = anonymizeString(a.Spec.Storage.GCS.KeyID)
	}
	if a.Spec.Storage.Swift != nil {
		a.Spec.Storage.Swift.AuthURL = anonymizeString(a.Spec.Storage.Swift.AuthURL)
		a.Spec.Storage.Swift.Container = anonymizeString(a.Spec.Storage.Swift.Container)
		a.Spec.Storage.Swift.Domain = anonymizeString(a.Spec.Storage.Swift.Domain)
		a.Spec.Storage.Swift.DomainID = anonymizeString(a.Spec.Storage.Swift.DomainID)
		a.Spec.Storage.Swift.Tenant = anonymizeString(a.Spec.Storage.Swift.Tenant)
		a.Spec.Storage.Swift.TenantID = anonymizeString(a.Spec.Storage.Swift.TenantID)
		a.Spec.Storage.Swift.RegionName = anonymizeString(a.Spec.Storage.Swift.RegionName)
	}
	return runtime.Encode(registrySerializer.LegacyCodec(registryv1.SchemeGroupVersion), a.Config)
}

// IngressAnonymizer implements serialization with marshalling
type IngressAnonymizer struct{ *configv1.Ingress }

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a IngressAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Ingress.Spec.Domain = anonymizeURL(a.Ingress.Spec.Domain)
	return runtime.Encode(openshiftSerializer, a.Ingress)
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
	return runtime.Encode(openshiftSerializer, a.Proxy)
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
	return runtime.Encode(openshiftSerializer, a.ClusterOperator)
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
