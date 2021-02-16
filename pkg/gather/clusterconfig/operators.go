package clusterconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/record/diskrecorder"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/check"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

const (
	// Log compression ratio is defining a multiplier for uncompressed logs
	// recorder would refuse to write files larger than MaxLogSize, so GatherClusterOperators
	// has to limit the expected size of the buffer for logs
	logCompressionRatio = 2
)

type clusterOperatorResource struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Name       string      `json:"name"`
	Spec       interface{} `json:"spec"`
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

// GatherClusterOperators collects all ClusterOperators and their resources.
// It finds unhealthy Pods for unhealthy operators
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusteroperator.go#L62
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusteroperatorlist-v1config-openshift-io
//
// Location of operators in archive: config/clusteroperator/
// See: docs/insights-archive-sample/config/clusteroperator
// Location of pods in archive: config/pod/
// Id in config: operators
func GatherClusterOperators(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherClusterOperators(g.ctx, gatherConfigClient, gatherKubeClient.CoreV1(), discoveryClient, dynamicClient)
	c <- gatherResult{records, errors}
}

func gatherClusterOperators(ctx context.Context, configClient configv1client.ConfigV1Interface, coreClient corev1client.CoreV1Interface, discoveryClient discovery.DiscoveryInterface, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	config, err := configClient.ClusterOperators().List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	resVer, _ := getOperatorResourcesVersions(discoveryClient)
	records := make([]record.Record, 0, len(config.Items))
	for idx, co := range config.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/clusteroperator/%s", config.Items[idx].Name),
			Item: record.JSONMarshaller{Object: &config.Items[idx]},
		})
		if resVer == nil {
			continue
		}
		relRes := collectClusterOperatorResources(ctx, dynamicClient, co, resVer)
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
			pods, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(2).Infof("Unable to find pods in namespace %s for failing operator %s", namespace, item.Name)
				continue
			}
			for j := range pods.Items {
				pod := &pods.Items[j]
				if check.IsHealthyPod(pod, now) {
					continue
				}
				records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name), Item: record.JSONMarshaller{Object: pod}})
				unhealthyPods = append(unhealthyPods, pod)
			}
			if namespaceEventsCollected.Has(namespace) {
				continue
			}
			namespaceRecords, errs := gatherNamespaceEvents(ctx, coreClient, namespace)
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
	bufferSize := int64(recorder.MaxLogSize * logCompressionRatio / totalUnhealthyContainers / 2)
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
				err = collectContainerLogs(ctx, coreClient, pod, buf, c.Name, isPrevious, &bufferSize)
				if err != nil {
					klog.V(2).Infof("Error: %q", err)
					continue
				}
				records = append(records, record.Record{Name: fmt.Sprintf("config/pod/%s/logs/%s/%s", pod.Namespace, pod.Name, logName), Item: marshal.Raw{Str: buf.String()}})
			}
		}
	}

	return records, nil
}

func gatherNamespaceEvents(ctx context.Context, coreClient corev1client.CoreV1Interface, namespace string) ([]record.Record, []error) {
	// do not accidentally collect events for non-openshift namespace
	if !strings.HasPrefix(namespace, "openshift-") {
		return []record.Record{}, nil
	}
	events, err := coreClient.Events(namespace).List(ctx, metav1.ListOptions{})
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
	return []record.Record{{Name: fmt.Sprintf("events/%s", namespace), Item: record.JSONMarshaller{Object: &compactedEvents}}}, nil
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
		versions := resVer[key]
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
				func() error { return utils.ParseJSONQuery(clusterResource.Object, "kind", &kind) },
				func() error { return utils.ParseJSONQuery(clusterResource.Object, "apiVersion", &apiVersion) },
				func() error { return utils.ParseJSONQuery(clusterResource.Object, "metadata.name", &name) },
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
				_, ok := resourceVersionMap[key]
				if !ok {
					resourceVersionMap[key] = []string{gv.Version}
					continue
				}
				resourceVersionMap[key] = append(resourceVersionMap[key], gv.Version)
			}
		}
	}
	return resourceVersionMap, nil
}

// collectContainerLogs fetches log lines from the pod
func collectContainerLogs(ctx context.Context, coreClient corev1client.CoreV1Interface, pod *corev1.Pod, buf *bytes.Buffer, containerName string, isPrevious bool, maxBytes *int64) error {
	req := coreClient.Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Previous: isPrevious, Container: containerName, LimitBytes: maxBytes, TailLines: &logTailLines})
	readCloser, err := req.Stream(ctx)
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
		resStr = strings.ReplaceAll(resStr, m, anonymize.AnonymizeString(m))
	}
	return []byte(resStr), nil
}

// GetExtension returns extension for anonymized cluster operator objects
func (a ClusterOperatorResourceAnonymizer) GetExtension() string {
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
