package conditional

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"regexp"
	"sync"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// recordWithError is a helper type to
// wrap the record with the corresponding error
type recordWithError struct {
	r   *record.Record
	err error
}

var sinceSeconds = int64(6 * 60 * 60)

// GatherContainersLogs TODO provide documentation for this function
func (g *Gatherer) GatherContainersLogs(rawLogRequests []RawLogRequest) (gatherers.GatheringClosure, error) { // nolint: dupl
	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			kubeConfigCopy := rest.CopyConfig(g.gatherProtoKubeConfig)
			kubeConfigCopy.Burst = 60
			kubeConfigCopy.QPS = 30
			kubeClient, err := kubernetes.NewForConfig(kubeConfigCopy)

			if err != nil {
				return nil, []error{err}
			}
			coreClient := kubeClient.CoreV1()
			return g.filterAndGatherContainerLogs(ctx, coreClient, rawLogRequests)
		},
	}, nil
}

func (g *Gatherer) filterAndGatherContainerLogs(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	rawLogRequests []RawLogRequest,
) ([]record.Record, []error) {
	var errs []error
	var records []record.Record

	namespaceToLogRequestMap := groupRawLogRequestsByNamespace(rawLogRequests)
	recCh := make(chan *recordWithError)
	go func() {
		for r := range recCh {
			if r.r != nil {
				records = append(records, *r.r)
			}
			if r.err != nil {
				errs = append(errs, r.err)
			}
		}
	}()

	var wg sync.WaitGroup
	for _, logRequest := range namespaceToLogRequestMap {
		klog.Infof("Start checking namespace %s for the Pod name pattern %s\n", logRequest.Namespace, logRequest.PodNameRegex)

		messagesRegex, err := listOfMessagesToRegex(logRequest.Messages)
		if err != nil {
			log.Default().Printf("Can't compile regex for %s: %v", logRequest.Namespace, err)
			continue
		}

		for podNameRegex := range logRequest.PodNameRegex {
			wg.Add(1)
			go filterAndGetLogsForPodContainers(ctx, coreClient, logRequest, podNameRegex, &wg, messagesRegex, recCh)
		}
	}
	wg.Wait()
	close(recCh)
	return records, errs
}

// groupRawLogRequestsByNamespace iterates over slice of the provided raw log requests and maps
// them with namespace name as the key and the logRequest as the value.
func groupRawLogRequestsByNamespace(rawLogRequests []RawLogRequest) map[string]LogRequest {
	namespaceToLogRequestMap := make(map[string]LogRequest, len(rawLogRequests))
	for _, logRequest := range rawLogRequests {
		existingLogRequest, ok := namespaceToLogRequestMap[logRequest.Namespace]

		if !ok {
			namespaceToLogRequestMap[logRequest.Namespace] = LogRequest{
				Namespace:    logRequest.Namespace,
				PodNameRegex: sets.Set[string](sets.NewString(logRequest.PodNameRegex)),
				Messages:     sets.Set[string](sets.NewString(logRequest.Messages...)),
				Previous:     logRequest.Previous,
			}
			continue
		}

		existingLogRequest.Messages = existingLogRequest.Messages.Union(sets.New[string](logRequest.Messages...))
		existingLogRequest.PodNameRegex = existingLogRequest.PodNameRegex.Union(sets.New[string](logRequest.PodNameRegex))
		namespaceToLogRequestMap[logRequest.Namespace] = existingLogRequest
	}
	return namespaceToLogRequestMap
}

// filterAndGetLogsForPodContainers compiles the provided podNameRegexStr and creates the map
// with Pod name as a key and slice of container names as a value. It iterates over the map and for each
// container name, it asynchronously (own Go routine) gets and filters the corresponding container
// log and the result is sent to the `recCh` channel.
func filterAndGetLogsForPodContainers(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	logRequest LogRequest,
	podNameRegexStr string,
	wg *sync.WaitGroup,
	messagesRegex *regexp.Regexp,
	recCh chan<- *recordWithError) {
	defer wg.Done()
	podNameRegex, err := regexp.Compile(podNameRegexStr)
	if err != nil {
		klog.Errorf("Failed to compile Pod name regular expression %s for the namespace %s: %v\n",
			logRequest.PodNameRegex, logRequest.Namespace, err)
		return
	}
	podToContainers, err := createPodToContainersMap(ctx, coreClient, logRequest.Namespace, podNameRegex)
	if err != nil {
		klog.Errorf("Failed to get matching pod names for namespace %s: %v\n", logRequest.Namespace, err)
		return
	}

	var wgContainers sync.WaitGroup
	for podName, containersaNames := range podToContainers {
		wgContainers.Add(len(containersaNames))
		for _, container := range containersaNames {
			containerLogReq := ContainerLogRequest{
				Namespace:     logRequest.Namespace,
				ContainerName: container,
				PodName:       podName,
				MessageRegex:  messagesRegex,
				Previous:      logRequest.Previous,
			}
			go func() {
				defer wgContainers.Done()
				rec, err := getAndFilterContainerLogs(ctx, coreClient, containerLogReq)
				recWithErr := &recordWithError{
					r:   rec,
					err: err,
				}
				recCh <- recWithErr
			}()
		}
	}
	wgContainers.Wait()
}

// getAndFilterContainerLogs reads the attributes of the provided ContainerLogRequest and
// based on the values it gets the corresponding container log and iterates over the log lines
// and tries to match the required container log messages.
func getAndFilterContainerLogs(ctx context.Context, coreClient corev1client.CoreV1Interface,
	containerLogRequest ContainerLogRequest) (*record.Record, error) {
	req := coreClient.Pods(containerLogRequest.Namespace).GetLogs(containerLogRequest.PodName, &corev1.PodLogOptions{
		Container:    containerLogRequest.ContainerName,
		SinceSeconds: &sinceSeconds,
		Timestamps:   true,
		Previous:     containerLogRequest.Previous,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	var byteBuffer bytes.Buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if containerLogRequest.MessageRegex.Match(line) {
			line = append(line, '\n')
			_, err = byteBuffer.Write(line)
			if err != nil {
				klog.Errorf("Failed to write line for container %s in the %s: %v",
					containerLogRequest.ContainerName, containerLogRequest.Namespace, err)
			}
		}
	}

	if len(byteBuffer.Bytes()) == 0 {
		return nil, nil
	}

	r := record.Record{
		Item: marshal.RawByte(byteBuffer.Bytes()),
		Name: fmt.Sprintf("namespaces/%s/pods/%s/%s/current.log",
			containerLogRequest.Namespace,
			containerLogRequest.PodName,
			containerLogRequest.ContainerName),
	}
	return &r, nil
}

// createPodToContainersMap lists all the Pods in the provided namespace
// and checks whether each Pod name matches the provided regular expression.
// If there is a match then add all the Pod related containers to the map.
// It returns map when key is the Pod name and the value is a slice of container names.
func createPodToContainersMap(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	namespace string,
	podNameRegex *regexp.Regexp) (map[string][]string, error) {
	podContainers := make(map[string][]string)

	podList, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for i := range podList.Items {
		pod := podList.Items[i]
		if podNameRegex.Match([]byte(pod.Name)) {
			var containerNames []string
			for i := range pod.Spec.Containers {
				c := pod.Spec.Containers[i]
				containerNames = append(containerNames, c.Name)
			}
			podContainers[pod.Name] = containerNames
		}
	}

	return podContainers, nil
}

// listOfMessagesToRegex takes the provided set of strings and each message
// is appended as "|" (or value) to the final regular expression, which is then compiled.
// It returns an error if the provided set is empty, nil or if the created regular expression
// cannot be compiled.
func listOfMessagesToRegex(messages sets.Set[string]) (*regexp.Regexp, error) {
	if len(messages) == 0 || messages == nil {
		return nil, fmt.Errorf("input messages are nil or empty")
	}

	messagesSlc := messages.UnsortedList()
	if len(messagesSlc) == 1 {
		regexStr := messagesSlc[0]
		return regexp.Compile(regexStr)
	}
	regexStr := messagesSlc[0]
	for i := 1; i < len(messagesSlc); i++ {
		m := messagesSlc[i]
		regexStr = fmt.Sprintf("%s|%s", regexStr, m)
	}
	return regexp.Compile(regexStr)
}
