package conditional

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/types"
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

// GatherContainersLogs refers to the Rapid Recommendations
// (see https://github.com/openshift/enhancements/blob/master/enhancements/insights/rapid-recommendations.md).
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
			return gatherContainerLogs(ctx, coreClient, rawLogRequests)
		},
	}, nil
}

func gatherContainerLogs(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	rawLogRequests []RawLogRequest,
) ([]record.Record, []error) {
	var errs []error
	var records []record.Record

	namespaceToLogRequestMap := groupRawLogRequestsByNamespace(rawLogRequests)
	recCh := make(chan *recordWithError)
	var receiveWG sync.WaitGroup
	receiveWG.Add(1)
	go func() {
		defer receiveWG.Done()
		for r := range recCh {
			if r.r != nil {
				records = append(records, *r.r)
			}
			if r.err != nil {
				errs = append(errs, r.err)
			}
		}
	}()

	// limiting the execution time of this gatherer to 5 minutes
	shorterCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	var sendWG sync.WaitGroup
	for _, logRequest := range namespaceToLogRequestMap {
		klog.Infof("Start checking namespace %s for the Pod name pattern %s\n", logRequest.Namespace, logRequest.PodNameRegex)

		for podNameRegex := range logRequest.PodNameRegex {
			sendWG.Add(1)
			go filterContainerLogs(shorterCtx, coreClient, logRequest, podNameRegex, &sendWG, recCh)
		}
	}
	sendWG.Wait()
	close(recCh)
	receiveWG.Wait()
	return records, errs
}

// filterContainerLogs compiles the provided podNameRegexStr and creates the map
// with Pod name as a key and slice of container names as a value. It iterates over the map and for each
// container name, it asynchronously (own Go routine) gets and filters the corresponding container
// log and the result is sent to the `recCh` channel.
func filterContainerLogs(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	logRequest LogRequest,
	podNameRegexStr string,
	wg *sync.WaitGroup,
	recCh chan<- *recordWithError) {
	defer wg.Done()

	messagesRegex, err := listOfMessagesToRegex(logRequest.Messages)
	if err != nil {
		errMessage := fmt.Sprintf("Failed to compile messages regular expression %s for %s namespace and Pod regexp %s: %v",
			messagesRegex,
			logRequest.Namespace,
			podNameRegexStr,
			err)
		klog.Warningf(errMessage)
		recCh <- &recordWithError{
			r:   nil,
			err: errors.New(errMessage),
		}
		return
	}

	podNameRegex, err := regexp.Compile(podNameRegexStr)
	if err != nil {
		errMessage := fmt.Sprintf("Failed to compile Pod name regular expression %s for %s namespace: %v",
			podNameRegexStr,
			logRequest.Namespace,
			err)
		klog.Warningf(errMessage)
		recCh <- &recordWithError{
			r:   nil,
			err: errors.New(errMessage),
		}
		return
	}
	podToContainers, err := createPodToContainersMap(ctx, coreClient, logRequest.Namespace, podNameRegex)
	if err != nil {
		klog.Errorf("Failed to get matching pod names for namespace %s: %v\n", logRequest.Namespace, err)
		return
	}

	var wgContainers sync.WaitGroup
	for podName, containerNames := range podToContainers {
		wgContainers.Add(len(containerNames))
		for _, container := range containerNames {
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
		warning := types.Warning{
			UnderlyingValue: fmt.Errorf("not found any data for the container %s in the Pod %s in the %s namespace",
				containerLogRequest.ContainerName,
				containerLogRequest.PodName,
				containerLogRequest.Namespace),
		}
		return nil, &warning
	}
	recordPath := fmt.Sprintf("namespaces/%s/pods/%s/%s", containerLogRequest.Namespace,
		containerLogRequest.PodName,
		containerLogRequest.ContainerName)

	recordName := fmt.Sprintf("%s/current.log", recordPath)
	if containerLogRequest.Previous {
		recordName = fmt.Sprintf("%s/previous.log", recordPath)
	}

	r := record.Record{
		Item: marshal.RawByte(byteBuffer.Bytes()),
		Name: recordName,
	}
	return &r, nil
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
		if podNameRegex.Match([]byte(pod.Name)) { //nolint:gocritic
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
