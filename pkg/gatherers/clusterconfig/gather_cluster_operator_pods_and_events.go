package clusterconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"errors"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"github.com/openshift/insights-operator/pkg/utils/check"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// CompactedEvent holds one Namespace Event
type CompactedEvent struct {
	Namespace     string    `json:"namespace"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message"`
	Type          string    `json:"type"`
}

// CompactedEventList is collection of events
type CompactedEventList struct {
	Items []CompactedEvent `json:"items"`
}

// Used to detect the possible stack trace on logs
var stackTraceRegex = regexp.MustCompile(`\.go:\d+\s\+0x`)

// GatherClusterOperatorPodsAndEvents Collects information about pods
// and events from namespaces of degraded cluster operators. The collected
// information includes:
//
// - Definitions for non-running (terminated, pending) Pods
// - Previous (if container was terminated) and current logs of all related pod containers
// - Namespace events
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/openshift-authentication-operator/authentication-operator-6d65456dc7-9d2qx.json
// - docs/insights-archive-sample/config/openshift-storage-operator/cluster-storage-operator-6974bfb5c6-tppp7.json
// - docs/insights-archive-sample/config/openshift-etcd-operator/etcd-operator-78bb597755-r6lgn.json
// - docs/insights-archive-sample/config/openshift-monitoring-operator/cluster-monitoring-operator-6c785d75f6-t79zv.json
//
// ### Location in archive
// - `config/pod/{namespace}/{pod}.json`
// - `events/{namespace}.json`
// - `config/pod/{namespace}/logs/{pod}/{container}_{current|previous}.log`
//
// ### Config ID
// `clusterconfig/operators_pods_and_events`
//
// ### Released version
// - 4.8.2
//
// ### Backported versions
// - 4.6.35+
// - 4.7.11+
//
// ### Changes
// - The data gathered by `ClusterOperatorPodsAndEvents` were originally gathered by
// [`ClusterOperators`](#ClusterOperators). The [`ClusterOperators`](#ClusterOperators) gather was split at 4.8.2
// and the change was backported to 4.7.11 and 4.6.35.
// - The collected data was previously included as specifications for `ClusterOperators`, and it was initially
// introduced in version `4.3.0` and later backported to version `4.2.10+`.
func (g *Gatherer) GatherClusterOperatorPodsAndEvents(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	interval := g.config().DataReporting.Interval
	records, err := gatherClusterOperatorPodsAndEvents(ctx, gatherConfigClient, gatherKubeClient.CoreV1(), interval)
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
	if kerrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// get all related pods for unhealthy operator
	pods, records, totalContainers := unhealthyClusterOperator(ctx, config.Items, coreClient, interval)
	// Exit early if no pods found
	klog.V(2).Infof("Found %d pods with %d containers", len(pods), totalContainers)
	if len(pods) == 0 || totalContainers <= 0 {
		return records, nil
	}

	bufferSize := int64(recorder.MaxArchiveSize * logCompressionRatio / totalContainers / 2)
	clogs, err := gatherPodsAndTheirContainersLogs(ctx, coreClient, pods, bufferSize)
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

			nsPods, nsTotal := getAllRelatedPods(podList.Items)
			pods = append(pods, nsPods...)
			totalContainers += nsTotal

			if namespaceEventsCollected.Has(namespace) {
				continue
			}

			eventRecords, err := gatherNamespaceEvents(ctx, coreClient, namespace, interval)
			if err != nil {
				klog.V(2).Infof("Unable to collect events for namespace %q: %#v", namespace, err)
				continue
			}

			records = append(records, eventRecords...)
			namespaceEventsCollected.Insert(namespace)
		}
	}

	return pods, records, totalContainers
}

// getAllRelatedPods collects all the cluster operator's related pods
// nolint: gocritic
func getAllRelatedPods(pods []corev1.Pod) ([]*corev1.Pod, int) {
	var podList []*corev1.Pod
	total := 0

	for j := range pods {
		pod := &pods[j]
		podList = append(podList, pod)
		total += len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
	}

	return podList, total
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
	filteredEvents := getEventsForInterval(interval, events)
	if len(filteredEvents.Items) == 0 {
		return nil, nil
	}
	compactedEvents := eventListToCompactedEventList(&filteredEvents)

	return []record.Record{{Name: fmt.Sprintf("events/%s", namespace), Item: record.JSONMarshaller{Object: &compactedEvents}}}, nil
}

// gatherPodsAndTheirContainersLogs iterates over all related pods and gets container
// log for every pod and tries to get previous log for every non-running/unhealthy pod. The definition
// of non-running/unhealthy pod is added to records as well.
func gatherPodsAndTheirContainersLogs(ctx context.Context,
	client corev1client.CoreV1Interface,
	pods []*corev1.Pod,
	bufferSize int64) ([]record.Record, error) {
	if bufferSize <= 0 {
		return nil, fmt.Errorf("invalid buffer size %d", bufferSize)
	}

	klog.V(2).Infof("Maximum buffer size: %v bytes", bufferSize)
	buf := bytes.NewBuffer(make([]byte, 0, bufferSize))

	// Fetch previous and current container logs
	var records []record.Record
	for _, pod := range pods {
		// if pod is not healthy then record its definition and try to get previous log
		if !check.IsHealthyPod(pod, time.Now()) {
			anonymize.SensitiveEnvVars(pod.Spec.Containers)

			records = append(records, record.Record{
				Name: fmt.Sprintf("config/pod/%s/%s", pod.Namespace, pod.Name),
				Item: record.ResourceMarshaller{Resource: pod},
			})
			previousLog := getContainerLogs(ctx, client, pod, true, buf)
			records = append(records, previousLog...)
		}
		currentLog := getContainerLogs(ctx, client, pod, false, buf)
		records = append(records, currentLog...)
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
	var stackTraceStart, stackTraceEnd int

	for index, line := range logArray {
		if found := stackTraceRegex.MatchString(line); found {
			stackTraceStart = index
			break
		}
	}

	for i := range logArray {
		index := len(logArray) - 1 - i
		line := logArray[index]
		if found := stackTraceRegex.MatchString(line); found {
			stackTraceEnd = index
			break
		}
	}

	if stackTraceEnd < stackTraceStart {
		stackTraceEnd = stackTraceStart
	}

	stackTraceLen := stackTraceEnd - stackTraceStart

	var result string

	if stackTraceLen > logStackTraceMaxLines {
		// add the beginning of the stacktrace
		from := utils.MaxInt(0, stackTraceStart-logLinesOffset)
		to := utils.MinInt(len(logArray), from+logStackTraceBeginningLimit)
		result = strings.Join(logArray[from:to], "\n")
		// add the message
		result += fmt.Sprintf("\n... (%v stacktrace lines suppressed) ...\n", stackTraceLen-logStackTraceMaxLines)
		// add the end of the stacktrace with all the following logs
		result += strings.Join(logArray[utils.MaxInt(0, stackTraceEnd-logStackTraceEndLimit):], "\n")
	} else {
		result = strings.Join(logArray[utils.MaxInt(0, stackTraceStart-logLinesOffset):], "\n")
	}

	return result
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
	if err != nil && !errors.Is(err, io.ErrShortBuffer) {
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
