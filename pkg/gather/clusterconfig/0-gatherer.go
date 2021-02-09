package clusterconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	apixv1beta1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	authv1client "github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1"
	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	"k8s.io/client-go/rest"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"
	securityv1client "github.com/openshift/client-go/security/clientset/versioned/typed/security/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	ctx             context.Context
	client          configv1client.ConfigV1Interface
	coreClient      corev1client.CoreV1Interface
	networkClient   networkv1client.NetworkV1Interface
	dynamicClient   dynamic.Interface
	metricsClient   rest.Interface
	certClient      certificatesv1beta1.CertificatesV1beta1Interface
	registryClient  imageregistryv1.ImageregistryV1Interface
	crdClient       apixv1beta1client.ApiextensionsV1beta1Interface
	policyClient    policyclient.PolicyV1beta1Interface
	lock            sync.Mutex
	lastVersion     *configv1.ClusterVersion
	discoveryClient discovery.DiscoveryInterface
	authClient      authv1client.AuthorizationV1Interface
	securityClient  securityv1client.SecurityV1Interface
}

// New creates new Gatherer
func New(client configv1client.ConfigV1Interface, coreClient corev1client.CoreV1Interface, certClient certificatesv1beta1.CertificatesV1beta1Interface, metricsClient rest.Interface,
	registryClient imageregistryv1.ImageregistryV1Interface, crdClient apixv1beta1client.ApiextensionsV1beta1Interface, networkClient networkv1client.NetworkV1Interface,
	dynamicClient dynamic.Interface, policyClient policyclient.PolicyV1beta1Interface, discoveryClient *discovery.DiscoveryClient, authClient authv1client.AuthorizationV1Interface, securityClient securityv1client.SecurityV1Interface) *Gatherer {
	return &Gatherer{
		client:          client,
		coreClient:      coreClient,
		certClient:      certClient,
		metricsClient:   metricsClient,
		registryClient:  registryClient,
		crdClient:       crdClient,
		networkClient:   networkClient,
		dynamicClient:   dynamicClient,
		policyClient:    policyClient,
		discoveryClient: discoveryClient,
		authClient:      authClient,
		securityClient:  securityClient,
	}
}

// Gather is hosting and calling all the recording functions
func (i *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	i.ctx = ctx
	return record.Collect(ctx, recorder,
		GatherPodDisruptionBudgets(i),
		GatherMostRecentMetrics(i),
		GatherClusterOperators(i),
		GatherContainerImages(i),
		GatherNodes(i),
		GatherConfigMaps(i),
		GatherClusterVersion(i),
		GatherClusterID(i),
		GatherClusterInfrastructure(i),
		GatherClusterNetwork(i),
		GatherClusterAuthentication(i),
		GatherClusterImageRegistry(i),
		GatherClusterImagePruner(i),
		GatherClusterFeatureGates(i),
		GatherClusterOAuth(i),
		GatherClusterIngress(i),
		GatherClusterProxy(i),
		GatherCertificateSigningRequests(i),
		GatherCRD(i),
		GatherHostSubnet(i),
		GatherMachineSet(i),
		GatherMachineConfigPool(i),
		GatherInstallPlans(i),
		GatherContainerRuntimeConfig(i),
		GatherOpenshiftSDNLogs(i),
		GatherNetNamespace(i),
		GatherServiceAccounts(i),
		GatherSAPConfig(i),
		GatherOpenshiftSDNControllerLogs(i),
	)
}

func (i *Gatherer) gatherNamespaceEvents(namespace string) ([]record.Record, []error) {
	// do not accidentally collect events for non-openshift namespace
	if !strings.HasPrefix(namespace, "openshift-") {
		return []record.Record{}, nil
	}
	events, err := i.coreClient.Events(namespace).List(i.ctx, metav1.ListOptions{})
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
