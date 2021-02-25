package clusterconfig

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apixv1beta1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	policyclient "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	configv1 "github.com/openshift/api/config/v1"
	registryv1 "github.com/openshift/api/imageregistry/v1"
	networkv1 "github.com/openshift/api/network/v1"
	openshiftscheme "github.com/openshift/client-go/config/clientset/versioned/scheme"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"github.com/openshift/library-go/pkg/image/reference"
	restclient "k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/record/diskrecorder"
)

const (
	// Log compression ratio is defining a multiplier for uncompressed logs
	// diskrecorder would refuse to write files larger than MaxLogSize, so GatherClusterOperators
	// has to limit the expected size of the buffer for logs
	logCompressionRatio = 2

	// imageGatherPodLimit is the maximum number of pods that
	// will be listed in a single request to reduce memory usage.
	imageGatherPodLimit = 200

	// containerImageLimit is the maximum number of container images to collect.
	// On average, information about one image takes up roughly 100 raw bytes.
	containerImageLimit = 1000

	// yyyyMmDateFormat is the date format used to get a YYYY-MM string.
	yyyyMmDateFormat = "2006-01"

	gatherPodDisruptionBudgetLimit = 5000

	// metricsAlertsLinesLimit is the maximal number of lines read from monitoring Prometheus
	// 500 KiB of alerts is limit, one alert line has typically 450 bytes => 1137 lines.
	// This number has been rounded to 1000 for simplicity.
	// Formerly, the `500 * 1024 / 450` expression was used instead.
	metricsAlertsLinesLimit = 1000

	// csrGatherLimit is the maximum number of crs that
	// will be listed in a single request to reduce memory usage.
	csrGatherLimit = 5000
	// InstallPlansTopX is the Maximal number of Install plans by non-unique instances count
	InstallPlansTopX = 100

	// Maximal total number of service accounts
	maxServiceAccountsLimit = 1000
	maxNamespacesLimit      = 1000
)

var (
	openshiftSerializer     = openshiftscheme.Codecs.LegacyCodec(configv1.SchemeGroupVersion)
	kubeSerializer          = kubescheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	policyV1Beta1Serializer = kubescheme.Codecs.LegacyCodec(policyv1beta1.SchemeGroupVersion)
	// maxEventTimeInterval represents the "only keep events that are maximum 1h old"
	// TODO: make this dynamic like the reporting window based on configured interval
	maxEventTimeInterval = 1 * time.Hour

	registrySerializer serializer.CodecFactory
	networkSerializer  serializer.CodecFactory
	registryScheme     = runtime.NewScheme()
	networkScheme      = runtime.NewScheme()

	// logTailLines sets maximum number of lines to fetch from pod logs
	logTailLines = int64(100)

	imageHostRegex = regexp.MustCompile(`(^|\.)(openshift\.org|registry\.redhat\.io|registry\.access\.redhat\.com)$`)

	// lineSep is the line separator used by the alerts metric
	lineSep = []byte{'\n'}
)

func init() {
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
	utilruntime.Must(networkv1.AddToScheme(networkScheme))
	networkSerializer = serializer.NewCodecFactory(networkScheme)
	registrySerializer = serializer.NewCodecFactory(registryScheme)
}

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
}

type clusterOperatorResource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Spec       interface{}
}

type netNamespace struct {
	Name      string                           `json:"name"`
	EgressIPs []networkv1.NetNamespaceEgressIP `json:"egressIPs"`
	NetID     uint32                           `json:"netID"`
}

// New creates new Gatherer
func New(client configv1client.ConfigV1Interface, coreClient corev1client.CoreV1Interface, certClient certificatesv1beta1.CertificatesV1beta1Interface, metricsClient rest.Interface,
	registryClient imageregistryv1.ImageregistryV1Interface, crdClient apixv1beta1client.ApiextensionsV1beta1Interface, networkClient networkv1client.NetworkV1Interface, dynamicClient dynamic.Interface, policyClient policyclient.PolicyV1beta1Interface, discoveryClient discovery.DiscoveryInterface) *Gatherer {
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
	}
}

var reInvalidUIDCharacter = regexp.MustCompile(`[^a-z0-9\-]`)

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
	)
}

// GatherPodDisruptionBudgets gathers the cluster's PodDisruptionBudgets.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/v11.0.0/kubernetes/typed/policy/v1beta1/poddisruptionbudget.go#L80
// Response see https://docs.okd.io/latest/rest_api/policy_apis/poddisruptionbudget-policy-v1beta1.html
//
// Location in archive: config/pdbs/
// See: docs/insights-archive-sample/config/pdbs
func GatherPodDisruptionBudgets(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		pdbs, err := i.policyClient.PodDisruptionBudgets("").List(i.ctx, metav1.ListOptions{Limit: gatherPodDisruptionBudgetLimit})
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, pdb := range pdbs.Items {
			recordName := fmt.Sprintf("config/pdbs/%s", pdb.GetName())
			if pdb.GetNamespace() != "" {
				recordName = fmt.Sprintf("config/pdbs/%s/%s", pdb.GetNamespace(), pdb.GetName())
			}
			records = append(records, record.Record{
				Name: recordName,
				Item: PodDisruptionBudgetsAnonymizer{&pdb},
			})
		}
		return records, nil
	}
}

// GatherMostRecentMetrics gathers cluster Federated Monitoring metrics.
//
// The GET REST query to URL /federate
// Gathered metrics:
//   etcd_object_counts
//   cluster_installer
//   namespace CPU and memory usage
//   followed by at most 1000 lines of ALERTS metric
//
// Location in archive: config/metrics/
// See: docs/insights-archive-sample/config/metrics
func GatherMostRecentMetrics(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		if i.metricsClient == nil {
			return nil, nil
		}
		data, err := i.metricsClient.Get().AbsPath("federate").
			Param("match[]", "etcd_object_counts").
			Param("match[]", "cluster_installer").
			Param("match[]", "namespace:container_cpu_usage_seconds_total:sum_rate").
			Param("match[]", "namespace:container_memory_usage_bytes:sum").
			DoRaw(i.ctx)
		if err != nil {
			// write metrics errors to the file format as a comment
			klog.Errorf("Unable to retrieve most recent metrics: %v", err)
			return []record.Record{{Name: "config/metrics", Item: RawByte(fmt.Sprintf("# error: %v\n", err))}}, nil
		}

		rsp, err := i.metricsClient.Get().AbsPath("federate").
			Param("match[]", "ALERTS").
			Stream(i.ctx)
		if err != nil {
			// write metrics errors to the file format as a comment
			klog.Errorf("Unable to retrieve most recent alerts from metrics: %v", err)
			return []record.Record{{Name: "config/metrics", Item: RawByte(fmt.Sprintf("# error: %v\n", err))}}, nil
		}
		r := NewLineLimitReader(rsp, metricsAlertsLinesLimit)
		alerts, err := ioutil.ReadAll(r)
		if err != nil && err != io.EOF {
			klog.Errorf("Unable to read most recent alerts from metrics: %v", err)
			return nil, []error{err}
		}

		remainingAlertLines, err := countLines(rsp)
		if err != nil && err != io.EOF {
			klog.Errorf("Unable to count truncated lines of alerts metric: %v", err)
			return nil, []error{err}
		}
		totalAlertCount := r.GetTotalLinesRead() + remainingAlertLines

		// # ALERTS <Total Alerts Lines>/<Alerts Line Limit>
		// The total number of alerts will typically be greater than the true number of alerts by 2
		// because the `# TYPE ALERTS untyped` header and the final empty line are counter in.
		data = append(data, []byte(fmt.Sprintf("# ALERTS %d/%d\n", totalAlertCount, metricsAlertsLinesLimit))...)
		data = append(data, alerts...)
		records := []record.Record{
			{Name: "config/metrics", Item: RawByte(data)},
		}

		return records, nil
	}
}

// GatherContainerImages collects essential information about running containers.
// Specifically, the age of pods, the set of running images and the container names are collected.
//
// Location in archive: config/running_containers.json
func GatherContainerImages(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		records := []record.Record{}

		// Cache for the temporary image count list.
		img2month2count := img2Month2CountMap{}

		// Use the Limit and Continue fields to request the pod information in chunks.
		continueValue := ""
		for {
			pods, err := i.coreClient.Pods("").List(i.ctx, metav1.ListOptions{
				Limit:    imageGatherPodLimit,
				Continue: continueValue,
				// FieldSelector: "status.phase=Running",
			})
			if err != nil {
				return nil, []error{err}
			}

			for podIndex, pod := range pods.Items {
				podPtr := &pods.Items[podIndex]
				if strings.HasPrefix(pod.Namespace, "openshift") && hasContainerInCrashloop(podPtr) {
					records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name), Item: PodAnonymizer{podPtr}})
				} else if pod.Status.Phase == corev1.PodRunning {
					startMonth := pod.CreationTimestamp.Time.UTC().Format(yyyyMmDateFormat)

					gatherImages(startMonth, img2month2count, pod.Status.ContainerStatuses)
					gatherImages(startMonth, img2month2count, pod.Status.InitContainerStatuses)
					gatherImages(startMonth, img2month2count, pod.Status.EphemeralContainerStatuses)
				}
			}

			// If the Continue field is not set, this should be the end of available data.
			// Otherwise, update the Continue value and perform another request iteration.
			if pods.Continue == "" {
				break
			}
			continueValue = pods.Continue
		}

		// Transform map into a list for sorting.
		imageCounts := []tmpImageCountEntry{}
		for img, countMap := range img2month2count {
			totalCount := 0
			for _, count := range countMap {
				totalCount += count
			}
			imageCounts = append(imageCounts, tmpImageCountEntry{
				Image:         img,
				TotalCount:    totalCount,
				CountPerMonth: countMap,
			})
		}

		// Sort images from most common to least common.
		sort.Slice(imageCounts, func(i, j int) bool {
			return imageCounts[i].TotalCount > imageCounts[j].TotalCount
		})

		// Reconstruct the image information into the reported data structure.
		contInfo := ContainerInfo{
			Images:     ContainerImageSet{},
			Containers: PodsWithAge{},
		}
		totalEntries := 0
		for _, img := range imageCounts {
			if totalEntries >= containerImageLimit {
				break
			}

			imgIndex := contInfo.Images.Add(img.Image)
			for month, count := range img.CountPerMonth {
				contInfo.Containers.Add(month, imgIndex, count)
				totalEntries++
			}
		}

		return append(records, record.Record{
			Name: "config/running_containers",
			Item: record.JSONMarshaller{Object: contInfo},
		}), nil
	}
}

// GatherClusterOperators collects all ClusterOperators and their resource.
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
		config, err := i.client.ClusterOperators().List(i.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		resVer, _ := getOperatorResourcesVersions(i.discoveryClient)
		records := make([]record.Record, 0, len(config.Items))
		for index, co := range config.Items {
			records = append(records, record.Record{Name: fmt.Sprintf("config/clusteroperator/%s", config.Items[index].Name), Item: ClusterOperatorAnonymizer{&config.Items[index]}})
			if resVer == nil {
				continue
			}
			relRes := collectClusterOperatorResources(i.ctx, i.dynamicClient, co, resVer)
			for _, rr := range relRes {
				// imageregistry resources (config, pruner) are gathered in image_registries.go, image_pruners.go
				if strings.Contains(rr.APIVersion, "imageregistry") {
					continue
				}
				gv, err := schema.ParseGroupVersion(rr.APIVersion)
				if err != nil {
					klog.Warningf("Unable to parse group version %s: %s", rr.APIVersion, err)
				}
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/clusteroperator/%s/%s/%s", gv.Group, strings.ToLower(rr.Kind), rr.Name),
					Item: ClusterOperatorResourceAnonymizer{rr},
				})
			}
		}
		namespaceEventsCollected := sets.NewString()
		now := time.Now()
		unhealthyPods := []*corev1.Pod{}
		for _, item := range config.Items {
			if isHealthyOperator(&item) {
				continue
			}
			for _, namespace := range namespacesForOperator(&item) {
				pods, err := i.coreClient.Pods(namespace).List(i.ctx, metav1.ListOptions{})
				if err != nil {
					klog.V(2).Infof("Unable to find pods in namespace %s for failing operator %s", namespace, item.Name)
					continue
				}
				for j := range pods.Items {
					pod := &pods.Items[j]
					if isHealthyPod(pod, now) {
						continue
					}
					records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name), Item: PodAnonymizer{pod}})
					unhealthyPods = append(unhealthyPods, pod)
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

		// Exit early if no unhealthy pods found
		if len(unhealthyPods) == 0 {
			return records, nil
		}

		// Fetch a list of containers in unhealthy pods and calculate a log size quota
		// Total log size must not exceed maxLogsSize multiplied by logCompressionRatio
		klog.V(2).Infof("Found %d unhealthy pods", len(unhealthyPods))
		totalUnhealthyContainers := 0
		for _, pod := range unhealthyPods {
			totalUnhealthyContainers += len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
		}
		bufferSize := int64(diskrecorder.MaxLogSize * logCompressionRatio / totalUnhealthyContainers / 2)
		klog.V(2).Infof("Maximum buffer size: %v bytes", bufferSize)
		buf := bytes.NewBuffer(make([]byte, 0, bufferSize))

		// Fetch previous and current container logs
		for _, isPrevious := range []bool{true, false} {
			for _, pod := range unhealthyPods {
				allContainers := pod.Spec.InitContainers
				allContainers = append(allContainers, pod.Spec.Containers...)
				for _, c := range allContainers {
					logName := fmt.Sprintf("%s_current.log", c.Name)
					if isPrevious {
						logName = fmt.Sprintf("%s_previous.log", c.Name)
					}
					buf.Reset()
					klog.V(2).Infof("Fetching logs for %s container %s pod in namespace %s (previous: %v): %v", c.Name, pod.Name, pod.Namespace, isPrevious, err)
					// Collect container logs and continue on error
					err = collectContainerLogs(i, pod, buf, c.Name, isPrevious, &bufferSize)
					if err != nil {
						klog.V(2).Infof("Error: %q", err)
						continue
					}
					records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/logs/%s/%s", pod.Namespace, pod.Name, logName), Item: Raw{buf.String()}})
				}
			}
		}

		return records, nil
	}
}

// collectContainerLogs fetches log lines from the pod
func collectContainerLogs(i *Gatherer, pod *corev1.Pod, buf *bytes.Buffer, containerName string, isPrevious bool, maxBytes *int64) error {
	req := i.coreClient.Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Previous: isPrevious, Container: containerName, LimitBytes: maxBytes, TailLines: &logTailLines})
	readCloser, err := req.Stream(i.ctx)
	if err != nil {
		klog.V(2).Infof("Failed to fetch log for %s pod in namespace %s for failing operator %s (previous: %v): %q", pod.Name, pod.Namespace, containerName, isPrevious, err)
		return err
	}

	defer readCloser.Close()

	_, err = io.Copy(buf, readCloser)
	if err != nil && err != io.ErrShortBuffer {
		klog.V(2).Infof("Failed to write log for %s pod in namespace %s for failing operator %s (previous: %v): %q", pod.Name, pod.Namespace, containerName, isPrevious, err)
		return err
	}
	return nil
}

func collectClusterOperatorResources(ctx context.Context, dynamicClient dynamic.Interface, co configv1.ClusterOperator, resVer map[string][]string) []clusterOperatorResource {
	var relObj []configv1.ObjectReference
	for _, ro := range co.Status.RelatedObjects {
		if strings.Contains(ro.Group, "operator.openshift.io") {
			relObj = append(relObj, ro)
		}
	}
	if len(relObj) == 0 {
		return nil
	}
	var res []clusterOperatorResource
	for _, ro := range relObj {
		key := fmt.Sprintf("%s-%s", ro.Group, strings.ToLower(ro.Resource))
		versions, _ := resVer[key]
		for _, v := range versions {
			gvr := schema.GroupVersionResource{Group: ro.Group, Version: v, Resource: strings.ToLower(ro.Resource)}
			clusterResource, err := dynamicClient.Resource(gvr).Get(ctx, ro.Name, metav1.GetOptions{})
			if err != nil {
				klog.V(2).Infof("Unable to list %s resource due to: %s", gvr, err)
			}
			if clusterResource == nil {
				continue
			}
			var kind, name, apiVersion string
			err = failEarly(
				func() error { return parseJSONQuery(clusterResource.Object, "kind", &kind) },
				func() error { return parseJSONQuery(clusterResource.Object, "apiVersion", &apiVersion) },
				func() error { return parseJSONQuery(clusterResource.Object, "metadata.name", &name) },
			)
			if err != nil {
				continue
			}
			spec, ok := clusterResource.Object["spec"]
			if !ok {
				klog.Warningf("Can't find spec for cluster operator resource %s", name)
			}
			res = append(res, clusterOperatorResource{Spec: spec, Kind: kind, Name: name, APIVersion: apiVersion})
		}
	}
	return res
}

func getOperatorResourcesVersions(discoveryClient discovery.DiscoveryInterface) (map[string][]string, error) {
	resources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	resourceVersionMap := make(map[string][]string)
	for _, v := range resources {
		if strings.Contains(v.GroupVersion, "operator.openshift.io") {
			gv, err := schema.ParseGroupVersion(v.GroupVersion)
			if err != nil {
				continue
			}
			for _, ar := range v.APIResources {
				key := fmt.Sprintf("%s-%s", gv.Group, ar.Name)
				r, ok := resourceVersionMap[key]
				if !ok {
					resourceVersionMap[key] = []string{gv.Version}
				} else {
					r = append(r, gv.Version)
				}
			}
		}
	}
	return resourceVersionMap, nil
}

// GatherNodes collects all Nodes.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/node.go#L78
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#nodelist-v1core
//
// Location in archive: config/node/
func GatherNodes(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		nodes, err := i.coreClient.Nodes().List(i.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, []error{err}
		}
		records := make([]record.Record, 0, len(nodes.Items))
		for i, node := range nodes.Items {
			records = append(records, record.Record{Name: fmt.Sprintf("config/node/%s", node.Name), Item: NodeAnonymizer{&nodes.Items[i]}})
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
		cms, err := i.coreClient.ConfigMaps("openshift-config").List(i.ctx, metav1.ListOptions{})
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
		config, err := i.client.ClusterVersions().Get(i.ctx, "version", metav1.GetOptions{})
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
		config, err := i.client.Infrastructures().Get(i.ctx, "cluster", metav1.GetOptions{})
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
		config, err := i.client.Networks().Get(i.ctx, "cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/network", Item: Anonymizer{config}}}, nil
	}
}

// GatherHostSubnet collects HostSubnet information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/hostsubnet.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#hostsubnet-v1-network-openshift-io
//
// Location in archive: config/hostsubnet/
func GatherHostSubnet(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {

		hsList, err := i.networkClient.HostSubnets().List(i.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := make([]record.Record, 0, len(hsList.Items))
		for i, h := range hsList.Items {
			records = append(records, record.Record{Name: fmt.Sprintf("config/hostsubnet/%s", h.Host), Item: HostSubnetAnonymizer{&hsList.Items[i]}})
		}
		return records, nil
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
		config, err := i.client.Authentications().Get(i.ctx, "cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		return []record.Record{{Name: "config/authentication", Item: Anonymizer{config}}}, nil
	}
}

// GatherClusterImagePruner fetches the image pruner configuration
//
// Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/imagepruner/cluster.json
func GatherClusterImagePruner(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		pruner, err := i.registryClient.ImagePruners().Get(i.ctx, "cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		// TypeMeta is empty - see https://github.com/kubernetes/kubernetes/issues/3030
		kinds, _, err := registryScheme.ObjectKinds(pruner)
		if err != nil {
			return nil, []error{err}
		}
		if len(kinds) > 1 {
			klog.Warningf("More kinds for image registry pruner operator resource %s", kinds)
		}
		objKind := kinds[0]
		return []record.Record{{
			Name: fmt.Sprintf("config/clusteroperator/%s/%s/%s", objKind.Group, strings.ToLower(objKind.Kind), pruner.Name),
			Item: ImagePrunerAnonymizer{pruner},
		}}, nil
	}
}

// GatherClusterImageRegistry fetches the cluster Image Registry configuration
// If the Image Registry configuration uses some PersistentVolumeClaim for the storage then the corresponding
// PersistentVolume definition is gathered
//
// Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/config/cluster.json
func GatherClusterImageRegistry(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		config, err := i.registryClient.Configs().Get(i.ctx, "cluster", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		// if there is some PVC then try to gather used persistent volume
		if config.Spec.Storage.PVC != nil {

			pvcName := config.Spec.Storage.PVC.Claim
			pv, err := findPVByPVCName(i.ctx, i.coreClient, pvcName)
			if err != nil {
				klog.Errorf("unable to find persistent volume: %s", err)
			} else {
				pvRecord := record.Record{
					Name: fmt.Sprintf("config/persistentvolumes/%s", pv.Name),
					Item: PersistentVolumeAnonymizer{pv},
				}
				records = append(records, pvRecord)
			}
		}
		// TypeMeta is empty - see https://github.com/kubernetes/kubernetes/issues/3030
		kinds, _, err := registryScheme.ObjectKinds(config)
		if err != nil {
			return nil, []error{err}
		}
		if len(kinds) > 1 {
			klog.Warningf("More kinds for image registry config operator resource %s", kinds)
		}
		objKind := kinds[0]
		coRecord := record.Record{
			Name: fmt.Sprintf("config/clusteroperator/%s/%s/%s", objKind.Group, strings.ToLower(objKind.Kind), config.Name),
			Item: ImageRegistryAnonymizer{config},
		}

		records = append(records, coRecord)
		return records, nil
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
		config, err := i.client.FeatureGates().Get(i.ctx, "cluster", metav1.GetOptions{})
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
		config, err := i.client.OAuths().Get(i.ctx, "cluster", metav1.GetOptions{})
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
		config, err := i.client.Ingresses().Get(i.ctx, "cluster", metav1.GetOptions{})
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
		config, err := i.client.Proxies().Get(i.ctx, "cluster", metav1.GetOptions{})
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
		requests, err := i.certClient.CertificateSigningRequests().List(i.ctx, metav1.ListOptions{
			Limit: csrGatherLimit,
		})
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

// GatherCRD collects the specified Custom Resource Definitions.
//
// The following CRDs are gathered:
// - volumesnapshots.snapshot.storage.k8s.io (10745 bytes)
// - volumesnapshotcontents.snapshot.storage.k8s.io (13149 bytes)
//
// The CRD sizes above are in the raw (uncompressed) state.
//
// Location in archive: config/crd/
func GatherCRD(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		toBeCollected := []string{
			"volumesnapshots.snapshot.storage.k8s.io",
			"volumesnapshotcontents.snapshot.storage.k8s.io",
		}
		records := []record.Record{}
		for _, crdName := range toBeCollected {
			crd, err := i.crdClient.CustomResourceDefinitions().Get(i.ctx, crdName, metav1.GetOptions{})
			// Log missing CRDs, but do not return the error.
			if errors.IsNotFound(err) {
				klog.V(2).Infof("Cannot find CRD: %q", crdName)
				continue
			}
			// Other errors will be returned.
			if err != nil {
				return []record.Record{}, []error{err}
			}
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/crd/%s", crd.Name),
				Item: record.JSONMarshaller{Object: crd},
			})
		}
		return records, []error{}
	}
}

//GatherMachineSet collects MachineSet information
//
// The Kubernetes api https://github.com/openshift/machine-api-operator/blob/master/pkg/generated/clientset/versioned/typed/machine/v1beta1/machineset.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#machineset-v1beta1-machine-openshift-io
//
// Location in archive: machinesets/
func GatherMachineSet(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinesets"}
		machineSets, err := i.dynamicClient.Resource(gvr).List(i.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, i := range machineSets.Items {
			recordName := fmt.Sprintf("machinesets/%s", i.GetName())
			if i.GetNamespace() != "" {
				recordName = fmt.Sprintf("machinesets/%s/%s", i.GetNamespace(), i.GetName())
			}
			records = append(records, record.Record{
				Name: recordName,
				Item: record.JSONMarshaller{Object: i.Object},
			})
		}
		return records, nil
	}
}

//GatherMachineConfigPool collects MachineConfigPool information
//
// The Kubernetes api https://github.com/openshift/machine-config-operator/blob/master/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L197
// Response see https://docs.okd.io/latest/rest_api/machine_apis/machineconfigpool-machineconfiguration-openshift-io-v1.html
//
// Location in archive: config/machineconfigpools/
func GatherMachineConfigPool(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		mcp := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"}
		machineCPs, err := i.dynamicClient.Resource(mcp).List(i.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, i := range machineCPs.Items {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/machineconfigpools/%s", i.GetName()),
				Item: record.JSONMarshaller{Object: i.Object},
			})
		}
		return records, nil
	}
}

// GatherContainerRuntimeConfig collects ContainerRuntimeConfig  information
//
// The Kubernetes api https://github.com/openshift/machine-config-operator/blob/master/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L402
// Response see https://docs.okd.io/latest/rest_api/machine_apis/containerruntimeconfig-machineconfiguration-openshift-io-v1.html
//
// Location in archive: config/containerruntimeconfigs/
func GatherContainerRuntimeConfig(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		crc := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "containerruntimeconfigs"}
		containerRCs, err := i.dynamicClient.Resource(crc).List(i.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, i := range containerRCs.Items {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/containerruntimeconfigs/%s", i.GetName()),
				Item: record.JSONMarshaller{Object: i.Object},
			})
		}
		return records, nil
	}
}

// GatherInstallPlans collects Top x InstallPlans from all openshift namespaces.
// Because InstallPlans have unique generated names, it groups them by namespace and the "template"
// for name generation from field generateName.
// It also collects Total number of all installplans and all non-unique installplans.
//
// The Operators-Framework api https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/installplan_types.go#L26
//
// Location in archive: config/installplans/
func GatherInstallPlans(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		var plansBatchLimit int64 = 500
		cont := ""
		recs := map[string]*collectedPlan{}
		total := 0
		opResource := schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "installplans"}

		config, err := getAllNamespaces(i)
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		// collect from all openshift* namespaces
		for _, ns := range config.Items {
			if !strings.HasPrefix(ns.Name, "openshift") {
				continue
			}

			resInterface := i.dynamicClient.Resource(opResource).Namespace(ns.Name)
			for {
				u, err := resInterface.List(i.ctx, metav1.ListOptions{Limit: plansBatchLimit, Continue: cont})
				if errors.IsNotFound(err) {
					return nil, nil
				}
				if err != nil {
					return nil, []error{err}
				}
				jsonMap := u.UnstructuredContent()
				var items []interface{}
				err = failEarly(
					func() error { return parseJSONQuery(jsonMap, "metadata.continue?", &cont) },
					func() error { return parseJSONQuery(jsonMap, "items", &items) },
				)
				if err != nil {
					return nil, []error{err}
				}
				total += len(items)
				for _, item := range items {
					if errs := collectInstallPlan(recs, item); errs != nil {
						return nil, errs
					}
				}

				if cont == "" {
					break
				}
			}
		}

		return []record.Record{{Name: "config/installplans", Item: InstallPlanAnonymizer{v: recs, total: total}}}, nil
	}
}

// GatherNetNamespace collects NetNamespaces networking information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/netnamespace.go
// Response is an array of netNamespaces. Netnamespace contains Name, EgressIPs and NetID attributes.
//
// Location in archive: config/netnamespaces
// Id in config: netnamespaces
func GatherNetNamespace(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		nsList, err := g.networkClient.NetNamespaces().List(g.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		namespaces := []*netNamespace{}
		for _, n := range nsList.Items {
			netNS := &netNamespace{
				Name:      n.Name,
				EgressIPs: n.EgressIPs,
				NetID:     n.NetID,
			}
			namespaces = append(namespaces, netNS)
		}
		r := record.Record{
			Name: "config/netnamespaces",
			Item: NetNamespaceAnonymizer{namespaces: namespaces},
		}
		return []record.Record{r}, nil
	}
}

func collectInstallPlan(recs map[string]*collectedPlan, item interface{}) []error {
	// Get common prefix
	csv := "[NONE]"
	var clusterServiceVersionNames []interface{}
	var ns, genName string
	var itemMap map[string]interface{}
	var ok bool
	if itemMap, ok = item.(map[string]interface{}); !ok {
		return []error{fmt.Errorf("cannot cast item to map %v", item)}
	}

	err := failEarly(
		func() error {
			return parseJSONQuery(itemMap, "spec.clusterServiceVersionNames", &clusterServiceVersionNames)
		},
		func() error { return parseJSONQuery(itemMap, "metadata.namespace", &ns) },
		func() error { return parseJSONQuery(itemMap, "metadata.generateName", &genName) },
	)
	if err != nil {
		return []error{err}
	}
	if len(clusterServiceVersionNames) > 0 {
		// ignoring non string
		csv, _ = clusterServiceVersionNames[0].(string)
	}

	key := fmt.Sprintf("%s.%s.%s", ns, genName, csv)
	m, ok := recs[key]
	if !ok {
		recs[key] = &collectedPlan{Namespace: ns, Name: genName, CSV: csv, Count: 1}
	} else {
		m.Count++
	}
	return nil
}

type collectedPlan struct {
	Namespace string
	Name      string
	CSV       string
	Count     int
}

// GatherOpenshiftSDNLogs collects logs from pods in openshift-sdn namespace with following substrings:
//   - "Got OnEndpointsUpdate for unknown Endpoints",
//   - "Got OnEndpointsDelete for unknown Endpoints",
//   - "Unable to update proxy firewall for policy",
//   - "Failed to update proxy firewall for policy",
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// Location in archive: config/pod/openshift-sdn/logs/{pod-name}/errors.log
func GatherOpenshiftSDNLogs(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		messagesToSearch := []string{
			"Got OnEndpointsUpdate for unknown Endpoints",
			"Got OnEndpointsDelete for unknown Endpoints",
			"Unable to update proxy firewall for policy",
			"Failed to update proxy firewall for policy",
		}

		records, err := gatherLogsFromPodsInNamespace(
			g.ctx,
			g.coreClient,
			"openshift-sdn",
			messagesToSearch,
			86400,   // last day
			1024*64, // maximum 64 kb of logs
			"errors",
			"app=sdn",
			false,
		)
		if err != nil {
			return nil, []error{err}
		}

		return records, nil
	}
}

// gatherLogsFromPodsInNamespace collects logs from the pods in provided namespace
//   - messagesToSearch are the messages to filter the logs(case-insensitive)
//   - sinceSeconds sets the moment to fetch logs from(current time - sinceSeconds)
//   - limitBytes sets the maximum amount of logs that can be fetched
//   - logFileName sets the name of the file to save logs to.
//   - labelSelector allows you to filter pods by their labels
//   - regexSearch makes messagesToSearch regex patterns, so you can accomplish more complicated search
//
// Location of the logs is `config/pod/{namespace}/logs/{podName}/{fileName}.log`
func gatherLogsFromPodsInNamespace(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	namespace string,
	messagesToSearch []string,
	sinceSeconds int64,
	limitBytes int64,
	logFileName string,
	labelSelector string,
	regexSearch bool,
) ([]record.Record, error) {
	pods, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	var records []record.Record

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			request := coreClient.Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container:    container.Name,
				SinceSeconds: &sinceSeconds,
				LimitBytes:   &limitBytes,
			})

			logs, err := filterLogs(ctx, request, messagesToSearch, regexSearch)
			if err != nil {
				return nil, err
			}

			if len(strings.TrimSpace(logs)) != 0 {
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/pod/%s/logs/%s/%s.log", pod.Namespace, pod.Name, logFileName),
					Item: Raw{logs},
				})
			}
		}
	}

	if len(pods.Items) == 0 {
		klog.Infof("no pods in %v namespace were found", namespace)
	}

	return records, nil
}

func filterLogs(
	ctx context.Context, request *restclient.Request, messagesToSearch []string, regexSearch bool,
) (string, error) {
	stream, err := request.Stream(ctx)
	if err != nil {
		return "", err
	}

	defer func() {
		err := stream.Close()
		if err != nil {
			klog.Errorf("error during closing a stream: %v", err)
		}
	}()

	scanner := bufio.NewScanner(stream)

	var result string

	for scanner.Scan() {
		line := scanner.Text()
		for _, messageToSearch := range messagesToSearch {
			if regexSearch {
				matches, err := regexp.MatchString(messageToSearch, line)
				if err != nil {
					return "", err
				}
				if matches {
					result += line + "\n"
				}
			} else {
				if strings.Contains(strings.ToLower(line), strings.ToLower(messageToSearch)) {
					result += line + "\n"
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return result, nil
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

func failEarly(fns ...func() error) error {
	for _, f := range fns {
		err := f()
		if err != nil {
			return err
		}
	}
	return nil
}

func parseJSONQuery(j map[string]interface{}, jq string, o interface{}) error {
	for _, k := range strings.Split(jq, ".") {
		// optional field
		opt := false
		sz := len(k)
		if sz > 0 && k[sz-1] == '?' {
			opt = true
			k = k[:sz-1]
		}

		if uv, ok := j[k]; ok {
			if v, ok := uv.(map[string]interface{}); ok {
				j = v
				continue
			}
			// ValueOf to enter reflect-land
			dstPtrValue := reflect.ValueOf(o)
			dstValue := reflect.Indirect(dstPtrValue)
			dstValue.Set(reflect.ValueOf(uv))

			return nil
		}
		if opt {
			return nil
		}
		// otherwise key was not found
		// keys are case sensitive, because maps are
		for ki := range j {
			if strings.ToLower(k) == strings.ToLower(ki) {
				return fmt.Errorf("key %s wasn't found, but %s was ", k, ki)
			}
		}
		return fmt.Errorf("key %s wasn't found in %v ", k, j)
	}
	return fmt.Errorf("query didn't match the structure")
}

func getAllNamespaces(i *Gatherer) (*corev1.NamespaceList, error) {
	ns, ok := i.ctx.Value(contextKeyAllNamespaces).(*corev1.NamespaceList)
	if ok {
		return ns, nil
	}
	ns, err := i.coreClient.Namespaces().List(i.ctx, metav1.ListOptions{Limit: maxNamespacesLimit})
	if err != nil {
		return nil, err
	}

	i.ctx = context.WithValue(i.ctx, contextKeyAllNamespaces, ns)

	return ns, nil
}

type contextKey string

const contextKeyAllNamespaces contextKey = "allnamespaces"

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

// Marshal serializes with OpenShift client-go openshiftSerializer
func (a Anonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, a.Object)
}

// GetExtension returns extension for anonymized openshift objects
func (a Anonymizer) GetExtension() string {
	return "json"
}

// InfrastructureAnonymizer anonymizes infrastructure
type InfrastructureAnonymizer struct{ *configv1.Infrastructure }

// Marshal serializes Infrastructure with anonymization
func (a InfrastructureAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, anonymizeInfrastructure(a.Infrastructure))
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
	return runtime.Encode(openshiftSerializer, a.ClusterVersion)
}

// GetExtension returns extension for anonymized cluster version objects
func (a ClusterVersionAnonymizer) GetExtension() string {
	return "json"
}

// FeatureGateAnonymizer implements serializaton of FeatureGate with anonymization
type FeatureGateAnonymizer struct{ *configv1.FeatureGate }

// Marshal serializes FeatureGate with anonymization
func (a FeatureGateAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, a.FeatureGate)
}

// GetExtension returns extension for anonymized feature gate objects
func (a FeatureGateAnonymizer) GetExtension() string {
	return "json"
}

// ImagePrunerAnonymizer implements serialization with marshalling
type ImagePrunerAnonymizer struct {
	*registryv1.ImagePruner
}

// Marshal serializes ImagePruner with anonymization
func (a ImagePrunerAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(registrySerializer.LegacyCodec(registryv1.SchemeGroupVersion), a.ImagePruner)
}

// GetExtension returns extension for anonymized image pruner objects
func (a ImagePrunerAnonymizer) GetExtension() string {
	return "json"
}

// ImageRegistryAnonymizer implements serialization with marshalling
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

// GetExtension returns extension for anonymized image registry objects
func (a ImageRegistryAnonymizer) GetExtension() string {
	return "json"
}

// IngressAnonymizer implements serialization with marshalling
type IngressAnonymizer struct{ *configv1.Ingress }

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a IngressAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Ingress.Spec.Domain = anonymizeURL(a.Ingress.Spec.Domain)
	return runtime.Encode(openshiftSerializer, a.Ingress)
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
	return runtime.Encode(openshiftSerializer, a.Proxy)
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
	return runtime.Encode(openshiftSerializer, a.ClusterOperator)
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

func anonymizeSliceOfStrings(slice []string) []string {
	anonymizedSlice := make([]string, len(slice), len(slice))
	for i, s := range slice {
		anonymizedSlice[i] = anonymizeString(s)
	}
	return anonymizedSlice
}

func isProductNamespacedKey(key string) bool {
	return strings.Contains(key, "openshift.io/") || strings.Contains(key, "k8s.io/") || strings.Contains(key, "kubernetes.io/")
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

type PodDisruptionBudgetsAnonymizer struct {
	*policyv1beta1.PodDisruptionBudget
}

// Marshal implements serialization of a PodDisruptionBudget with anonymization
func (a PodDisruptionBudgetsAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(policyV1Beta1Serializer, a.PodDisruptionBudget)
}

// GetExtension returns extension for anonymized PodDisruptionBudget objects
func (a PodDisruptionBudgetsAnonymizer) GetExtension() string {
	return "json"
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

// GetExtension returns extension for anonymized openshift objects
func (a ConfigMapAnonymizer) GetExtension() string {
	return ""
}

// HostSubnetAnonymizer implements HostSubnet serialization
type HostSubnetAnonymizer struct{ *networkv1.HostSubnet }

// Marshal implements HostSubnet serialization
func (a HostSubnetAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(networkSerializer.LegacyCodec(networkv1.SchemeGroupVersion), a.HostSubnet)
}

// GetExtension returns extension for HostSubnet object
func (a HostSubnetAnonymizer) GetExtension() string {
	return "json"
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

// RunningImages assigns information about running containers to a specific image index.
// The index is a reference to an item in the related `ContainerImageSet` instance.
type RunningImages map[int]int

// PodsWithAge maps the YYYY-MM string representation of start time to list of pods running since that month.
type PodsWithAge map[string]RunningImages

// Add inserts the specified container information into the data structure.
func (p PodsWithAge) Add(startMonth string, image int, count int) {
	if imageMap, exists := p[startMonth]; exists {
		if _, exists := imageMap[image]; exists {
			imageMap[image] += count
		} else {
			imageMap[image] = count
		}
	} else {
		p[startMonth] = RunningImages{image: count}
	}
}

// ContainerImageSet is used to store unique container image URLs.
// The key is a continuous index starting from 0.
// The value is the image URL itself.
type ContainerImageSet map[int]string

// Add puts the image at the end of the set.
// It will be assigned the highest index and this index will be returned.
func (is ContainerImageSet) Add(image string) int {
	nextIndex := len(is)
	is[nextIndex] = image
	return nextIndex
}

// ContainerInfo encapsulates the essential information about running containers in a minimalized data structure.
type ContainerInfo struct {
	Images     ContainerImageSet `json:"images"`
	Containers PodsWithAge       `json:"containers"`
}

type img2Month2CountMap map[string]map[string]int

type tmpImageCountEntry struct {
	Image         string
	CountPerMonth map[string]int
	TotalCount    int
}

func gatherImages(startMonth string, img2month2count img2Month2CountMap, containers []corev1.ContainerStatus) {
	for _, container := range containers {
		dockerRef, err := reference.Parse(container.Image)
		if err != nil {
			klog.Warningf("Unable to parse container image specification: %v", err)
			continue
		}

		// Use the sha256 hash ID if available, otherwise use the full image spec.
		imgMinimal := dockerRef.ID
		if imgMinimal == "" {
			imgMinimal = container.Image
		}

		if countMap, ok := img2month2count[imgMinimal]; ok {
			var count int
			if count, ok = countMap[startMonth]; !ok {
				count = 0
			}
			countMap[startMonth] = count + 1
		} else {
			img2month2count[imgMinimal] = map[string]int{
				startMonth: 1,
			}
		}
	}
}

func isContainerInCrashloop(status *corev1.ContainerStatus) bool {
	return status.RestartCount > 0 && ((status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0) || status.LastTerminationState.Waiting != nil)
}

func hasContainerInCrashloop(pod *corev1.Pod) bool {
	for _, status := range pod.Status.InitContainerStatuses {
		if isContainerInCrashloop(&status) {
			return true
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if isContainerInCrashloop(&status) {
			return true
		}
	}
	return false
}

// NewLineLimitReader returns a Reader that reads from `r` but stops with EOF after `n` lines.
func NewLineLimitReader(r io.Reader, n int) *LineLimitedReader { return &LineLimitedReader{r, n, 0} }

// A LineLimitedReader reads from R but limits the amount of
// data returned to just N lines. Each call to Read
// updates N to reflect the new amount remaining.
// Read returns EOF when N <= 0 or when the underlying R returns EOF.
type LineLimitedReader struct {
	reader        io.Reader // underlying reader
	maxLinesLimit int       // max lines remaining
	// totalLinesRead is the total number of line separators already read by the underlying reader.
	totalLinesRead int
}

func (l *LineLimitedReader) Read(p []byte) (int, error) {
	if l.maxLinesLimit <= 0 {
		return 0, io.EOF
	}

	rc, err := l.reader.Read(p)
	l.totalLinesRead += bytes.Count(p[:rc], lineSep)

	lc := 0
	for {
		lineSepIdx := bytes.Index(p[lc:rc], lineSep)
		if lineSepIdx == -1 {
			return rc, err
		}
		if l.maxLinesLimit <= 0 {
			return lc, io.EOF
		}
		l.maxLinesLimit--
		lc += lineSepIdx + 1 // skip past the EOF
	}
}

// GetTotalLinesRead return the total number of line separators already read by the underlying reader.
// This includes lines that have been truncated by the `Read` calls after exceeding the line limit.
func (l *LineLimitedReader) GetTotalLinesRead() int { return l.totalLinesRead }

// countLines reads the remainder of the reader and counts the number of lines.
//
// Inspired by: https://stackoverflow.com/a/24563853/
func countLines(r io.Reader) (int, error) {
	buf := make([]byte, 0x8000)
	// Original implementation started from 0, but a file with no line separator
	// still contains a single line, so I would say that was an off-by-1 error.
	lineCount := 1
	for {
		c, err := r.Read(buf)
		lineCount += bytes.Count(buf[:c], lineSep)
		if err != nil {
			return lineCount, err
		}
	}
}

// ClusterOperatorResourceAnonymizer implements serialization of clusterOperatorResource
type ClusterOperatorResourceAnonymizer struct{ resource clusterOperatorResource }

// Marshal serializes clusterOperatorResource with IP address anonymization
func (a ClusterOperatorResourceAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	bytes, err := json.Marshal(a.resource)
	if err != nil {
		return nil, err
	}
	resStr := string(bytes)
	//anonymize URLs
	re := regexp.MustCompile(`"(https|http)://(.*?)"`)
	urlMatches := re.FindAllString(resStr, -1)
	for _, m := range urlMatches {
		m = strings.ReplaceAll(m, "\"", "")
		resStr = strings.ReplaceAll(resStr, m, anonymizeString(m))
	}
	// anonymize IP addresses
	re = regexp.MustCompile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	ipMatches := re.FindAllString(resStr, -1)
	for _, m := range ipMatches {
		resStr = strings.ReplaceAll(resStr, m, anonymizeString(m))
	}
	return []byte(resStr), nil
}

// GetExtension returns extension for anonymized cluster operator objects
func (a ClusterOperatorResourceAnonymizer) GetExtension() string {
	return "json"
}

// InstallPlanAnonymizer implements serialization of top x installplans
type InstallPlanAnonymizer struct {
	v     map[string]*collectedPlan
	total int
	limit int
}

// Marshal implements serialization of InstallPlan
func (a InstallPlanAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	if a.limit == 0 {
		a.limit = InstallPlansTopX
	}
	cnts := []int{}
	for _, v := range a.v {
		cnts = append(cnts, v.Count)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(cnts)))
	countLimit := -1
	if len(cnts) > a.limit && a.limit > 0 {
		// nth plan is on n-1th position
		countLimit = cnts[a.limit-1]
	}
	// Creates map for marshal
	sr := map[string]interface{}{}
	st := map[string]int{}
	st["TOTAL_COUNT"] = a.total
	st["TOTAL_NONUNIQ_COUNT"] = len(a.v)
	sr["stats"] = st
	uls := 0
	it := []interface{}{}
	for _, v := range a.v {
		if v.Count >= countLimit {
			kvp := map[string]interface{}{}
			kvp["ns"] = v.Namespace
			kvp["name"] = v.Name
			kvp["csv"] = v.CSV
			kvp["count"] = v.Count
			it = append(it, kvp)
			uls++
		}
		if uls >= a.limit {
			break
		}
	}
	sort.SliceStable(it, func(i, j int) bool {
		return it[i].(map[string]interface{})["count"].(int) > it[j].(map[string]interface{})["count"].(int)
	})
	sr["items"] = it
	return json.Marshal(sr)
}

// GetExtension returns extension for anonymized openshift objects
func (a InstallPlanAnonymizer) GetExtension() string {
	return "json"
}

// findPVByPVCName tries to find *corev1.PersistentVolume used in PersistentVolumeClaim with provided name
func findPVByPVCName(ctx context.Context, coreClient corev1client.CoreV1Interface, name string) (*corev1.PersistentVolume, error) {
	// unfortunately we can't do "coreClient.PersistentVolumeClaims("").Get(ctx, name, ... )"
	pvcs, err := coreClient.PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var pvc *corev1.PersistentVolumeClaim
	for _, p := range pvcs.Items {
		if p.Name == name {
			pvc = &p
			break
		}
	}
	if pvc == nil {
		return nil, fmt.Errorf("can't find any %s persistentvolumeclaim", name)
	}
	pvName := pvc.Spec.VolumeName
	pv, err := coreClient.PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

// PersistentVolumeAnonymizer implements serialization with marshalling
type PersistentVolumeAnonymizer struct {
	*corev1.PersistentVolume
}

// Marshal implements serialization of corev1.PersistentVolume without anonymization
func (p PersistentVolumeAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, p.PersistentVolume)
}

// GetExtension returns extension for PersistentVolume objects
func (p PersistentVolumeAnonymizer) GetExtension() string {
	return "json"
}

// NetNamespaceAnonymizer implements NetNamespace serialization
type NetNamespaceAnonymizer struct{ namespaces []*netNamespace }

// Marshal implements NetNamespace serialization
func (a NetNamespaceAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(a.namespaces)
}

// GetExtension returns extension for NetNamespace object
func (a NetNamespaceAnonymizer) GetExtension() string {
	return "json"
}
