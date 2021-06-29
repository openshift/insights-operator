package clusterconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/utils/check"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

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

// Used to detect the possible stack trace on logs
var stackTraceRegex = regexp.MustCompile(`\.go:\d+\s\+0x`)

// GatherClusterOperatorPodsAndEvents collects information about all pods
// and events from namespaces of degraded cluster operators. The collected
// information includes:
//
// - Pod definitions
// - Previous and current logs of pod containers (when available)
// - Namespace events
//
// * Location of pod definitions: config/pod/{namespace}/{pod}.json
// * Location of pod container current logs:
//   config/pod/{namespace}/logs/{pod}/{container}_current.log
// * Location of pod container previous logs:
//   config/pod/{namespace}/logs/{pod}/{container}_previous.log
// * Location of events in archive: events/
// * Id in config: operators_pods_and_events
// * Spec config for CO resources since versions:
//   * 4.6.16+
//   * 4.7+
func (g *Gatherer) GatherClusterOperatorPodsAndEvents(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	records, err := gatherClusterOperatorPodsAndEvents(ctx, gatherConfigClient, gatherKubeClient.CoreV1(), g.interval)
	if err != nil {
		return records, []error{err}
	}

	return records, nil
}

func gatherClusterOperatorPodsAndEvents(ctx context.Context,
	configClient configv1client.ConfigV1Interface,
	coreClient corev1client.CoreV1Interface,
	interval time.Duration) ([]record.Record, error) {
	config, err := configClient.ClusterOperators().List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// gather pods from unhealthy operators
	pods, records, totalContainers := unhealthyClusterOperator(ctx, config.Items, coreClient, interval)
	// Exit early if no pods found
	klog.V(2).Infof("Found %d pods with %d containers", len(pods), totalContainers)
	if len(pods) == 0 || totalContainers <= 0 {
		return records, nil
	}

	// gather pods containers logs
	bufferSize := int64(recorder.MaxArchiveSize * logCompressionRatio / totalContainers / 2)
	clogs, err := gatherPodContainersLogs(ctx, coreClient, pods, bufferSize)
	if err != nil {
		klog.V(2).Infof("Unable to gather pod containers logs: %v", err)
		return records, nil
	}

	if len(clogs) > 0 {
		records = append(records, clogs...)
	}

	return records, nil
}

// unhealthyClusterOperator collects unhealthy cluster operator resources
// nolint: gocritic, gosec
func unhealthyClusterOperator(ctx context.Context,
	items []configv1.ClusterOperator,
	coreClient corev1client.CoreV1Interface,
	interval time.Duration) ([]*corev1.Pod, []record.Record, int) {
	var records []record.Record

	namespaceEventsCollected := sets.NewString()
	pods := []*corev1.Pod{}
	totalContainers := 0

	for _, item := range items {
		if isHealthyOperator(&item) {
			continue
		}

		for _, namespace := range namespacesForOperator(&item) {
			podList, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(2).Infof("Unable to find pods in namespace %s for failing operator %s", namespace, item.Name)
				continue
			}

			uhPods, uhRecords, uhTotal := gatherUnhealthyPods(podList.Items)
			pods = append(pods, uhPods...)
			records = append(records, uhRecords...)
			totalContainers += uhTotal

			if namespaceEventsCollected.Has(namespace) {
				continue
			}

			namespaceRecords, err := gatherNamespaceEvents(ctx, coreClient, namespace, interval)
			if err != nil {
				klog.V(2).Infof("Unable to collect events for namespace %q: %#v", namespace, err)
				continue
			}

			records = append(records, namespaceRecords...)
			namespaceEventsCollected.Insert(namespace)
		}
	}

	return pods, records, totalContainers
}

// gatherUnhealthyPods collects cluster operator unhealthy pods
func gatherUnhealthyPods(pods []corev1.Pod) ([]*corev1.Pod, []record.Record, int) {
	var records []record.Record
	var podList []*corev1.Pod
	total := 0
	now := time.Now()

	for j := range pods {
		pod := &pods[j]
		if check.IsHealthyPod(pod, now) {
			continue
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name),
			Item: record.ResourceMarshaller{Resource: pod},
		})
		podList = append(podList, pod)
		total += len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
	}

	return podList, records, total
}

// gatherNamespaceEvents gather all namespace events
func gatherNamespaceEvents(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	namespace string,
	interval time.Duration) ([]record.Record, error) {
	// do not accidentally collect events for non-openshift namespace
	if !strings.HasPrefix(namespace, "openshift-") {
		return []record.Record{}, nil
	}
	events, err := coreClient.Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// filter the event list to only recent events
	oldestEventTime := time.Now().Add(-interval)
	var filteredEventIndex []int
	for i := range events.Items {
		// if LastTimestamp is zero then try to check the event series
		if events.Items[i].LastTimestamp.IsZero() {
			if events.Items[i].Series != nil {
				if events.Items[i].Series.LastObservedTime.Time.After(oldestEventTime) {
					filteredEventIndex = append(filteredEventIndex, i)
				}
			}
		} else {
			if events.Items[i].LastTimestamp.Time.After(oldestEventTime) {
				filteredEventIndex = append(filteredEventIndex, i)
			}
		}
	}
	if len(filteredEventIndex) == 0 {
		return nil, nil
	}
	compactedEvents := CompactedEventList{Items: make([]CompactedEvent, len(filteredEventIndex))}
	for i, index := range filteredEventIndex {
		compactedEvents.Items[i] = CompactedEvent{
			Namespace:     events.Items[index].Namespace,
			LastTimestamp: events.Items[index].LastTimestamp.Time,
			Reason:        events.Items[index].Reason,
			Message:       events.Items[index].Message,
		}
		if events.Items[index].LastTimestamp.Time.IsZero() {
			compactedEvents.Items[i].LastTimestamp = events.Items[index].Series.LastObservedTime.Time
		}
	}
	sort.Slice(compactedEvents.Items, func(i, j int) bool {
		return compactedEvents.Items[i].LastTimestamp.Before(compactedEvents.Items[j].LastTimestamp)
	})

	return []record.Record{{Name: fmt.Sprintf("events/%s", namespace), Item: record.JSONMarshaller{Object: &compactedEvents}}}, nil
}

// gatherPodContainersLogs collect the pod current and previous containers logs
func gatherPodContainersLogs(ctx context.Context,
	client corev1client.CoreV1Interface,
	pods []*corev1.Pod,
	bufferSize int64) ([]record.Record, error) {
	if bufferSize <= 0 {
		return nil, fmt.Errorf("invalid buffer size %d", bufferSize)
	}

	// Fetch a list of containers in pods and calculate a log size quota
	// Total log size must not exceed maxLogsSize multiplied by logCompressionRatio
	klog.V(2).Infof("Maximum buffer size: %v bytes", bufferSize)
	buf := bytes.NewBuffer(make([]byte, 0, bufferSize))

	// Fetch previous and current container logs
	var records []record.Record
	for _, isPrevious := range []bool{true, false} {
		for _, pod := range pods {
			clog := getContainerLogs(ctx, client, pod, isPrevious, buf)
			if len(clog) > 0 {
				records = append(records, clog...)
			}
		}
	}

	return records, nil
}

// getContainerLogs get previous and current log reports for pod containers using the k8s API response
// nolint: gocritic
func getContainerLogs(ctx context.Context,
	client corev1client.CoreV1Interface,
	pod *corev1.Pod,
	isPrevious bool,
	buf *bytes.Buffer) []record.Record {
	var records []record.Record

	allContainers := append(pod.Spec.InitContainers, pod.Spec.Containers...)
	for _, c := range allContainers {
		// only grab previous log if the pod is restarted
		if isPrevious && !isPodRestarted(pod) {
			continue
		}

		logName := logFilename(c.Name, isPrevious)

		// Fetch container logs and continue on error
		logString, err := getContainerLogString(ctx, client, pod, c.Name, isPrevious, buf, &logMaxTailLines)
		if err != nil {
			klog.V(2).Infof("Error: %q", err)
			continue
		}

		if found := stackTraceRegex.MatchString(logString); found {
			klog.V(2).Infof(
				"Stack trace found in log for %s container %s pod in namespace %s (previous: %v).",
				c.Name,
				pod.Name,
				pod.Namespace,
				isPrevious)
			logString, err = getContainerLogString(ctx, client, pod, c.Name, isPrevious, buf, &logMaxLongTailLines)
			if err != nil {
				klog.V(2).Infof("Error: %q", err)
				continue
			}
			// find the stack trace and get its
			logString = getLogWithStacktracing(strings.Split(logString, "\n"))
		}

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/pod/%s/logs/%s/%s", pod.Namespace, pod.Name, logName),
			Item: marshal.Raw{Str: logString},
		})
	}

	return records
}

// getLogWithStacktracing search for the first stack trace line and offset it by logLinesOffset
func getLogWithStacktracing(logArray []string) string {
	var limit int
	for idx := range logArray {
		line := logArray[idx]
		if found := stackTraceRegex.MatchString(line); found {
			limit = idx - logLinesOffset
			if limit < 0 {
				limit = 0
			}
			break
		}
	}
	return strings.Join(logArray[limit:], "\n")
}

// getContainerLogString fetch the container log from API and return it as String
func getContainerLogString(
	ctx context.Context,
	client corev1client.CoreV1Interface,
	pod *corev1.Pod,
	name string,
	isPrevious bool,
	buf *bytes.Buffer,
	tailLines *int64) (string, error) {
	// Reset the given buffer
	buf.Reset()

	klog.V(2).Infof("Fetching logs for %s container %s pod in namespace %s (previous: %v).", name, pod.Name, pod.Namespace, isPrevious)
	err := fetchPodContainerLog(ctx, client, pod, buf, name, isPrevious, tailLines)
	if err != nil {
		return "", err
	}

	if buf.Len() == 0 {
		return "", fmt.Errorf("log buffer is empty")
	}

	return buf.String(), nil
}

// logFilename creates the filename to Pod logs
func logFilename(name string, prev bool) string {
	filename := fmt.Sprintf("%s_current.log", name)
	if prev {
		filename = fmt.Sprintf("%s_previous.log", name)
	}
	return filename
}

// fetchPodContainerLog fetches log lines from the pod
func fetchPodContainerLog(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	pod *corev1.Pod,
	buf *bytes.Buffer,
	containerName string,
	isPrevious bool,
	tailLines *int64) error {
	var limitBytes *int64
	bufCap := int64(buf.Cap())
	limitBytes = &bufCap

	req := coreClient.Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Previous:   isPrevious,
		Container:  containerName,
		TailLines:  tailLines,
		LimitBytes: limitBytes,
		Timestamps: true,
	})
	readCloser, err := req.Stream(ctx)
	if err != nil {
		klog.V(2).Infof("Failed to fetch log for %s pod in namespace %s for failing operator %s (previous: %v): %q",
			pod.Name,
			pod.Namespace,
			containerName,
			isPrevious,
			err)
		return err
	}

	defer readCloser.Close()

	_, err = io.Copy(buf, readCloser)
	if err != nil && err != io.ErrShortBuffer {
		klog.V(2).Infof("Failed to write log for %s pod in namespace %s for failing operator %s (previous: %v): %q",
			pod.Name,
			pod.Namespace,
			containerName,
			isPrevious,
			err)
		return err
	}
	return nil
}

// isPodRestarted checks if pod was restarted by testing its container's restart count status is bigger than zero
// nolint: gocritic
func isPodRestarted(pod *corev1.Pod) bool {
	// pods that have containers that have terminated with non-zero exit codes are considered failure
	for _, status := range pod.Status.InitContainerStatuses {
		if status.RestartCount > 0 {
			return true
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.RestartCount > 0 {
			return true
		}
	}
	return false
}

// isHealthyOperator checks if operator ins't degraded or unavailable
func isHealthyOperator(operator *configv1.ClusterOperator) bool {
	for _, condition := range operator.Status.Conditions {
		if isOperatorConditionDegraded(&condition) || isOperatorConditionAvailable(&condition) { //nolint: gosec
			return false
		}
	}
	return true
}

// isOperatorConditionDegraded check if the operator status condition degraded is true
func isOperatorConditionDegraded(c *configv1.ClusterOperatorStatusCondition) bool {
	return c.Type == configv1.OperatorDegraded && c.Status == configv1.ConditionTrue
}

// isOperatorConditionAvailable check if the operator status condition available is false
func isOperatorConditionAvailable(c *configv1.ClusterOperatorStatusCondition) bool {
	return c.Type == configv1.OperatorAvailable && c.Status == configv1.ConditionFalse
}

// namespacesForOperator get all the cluster operator namespaces
func namespacesForOperator(operator *configv1.ClusterOperator) []string {
	var ns []string
	for _, ref := range operator.Status.RelatedObjects {
		if ref.Resource == "namespaces" {
			ns = append(ns, ref.Name)
		}
	}
	return ns
}
