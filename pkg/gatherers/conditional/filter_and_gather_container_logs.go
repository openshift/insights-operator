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
	"github.com/openshift/insights-operator/pkg/utils"
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

// GatherContainersLogs is used for more dynamic log gathering based on the
// [Rapid Recommendations](https://github.com/openshift/enhancements/blob/master/enhancements/insights/rapid-recommendations.md).
//
// In general this function finds the Pods (and containers) that match the requested data and filters all the container logs
// to match the specific messages up to a maximum of 6 hours old.
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
		sendWG.Add(1)
		klog.Infof("Start checking namespace %s for the Pod name pattern %s\n",
			logRequest.Namespace,
			mapKeysToSlice(logRequest.PodNameRegexToMessages))
		go filterContainerLogs(shorterCtx, coreClient, logRequest, &sendWG, recCh)
	}
	sendWG.Wait()
	close(recCh)
	receiveWG.Wait()
	return records, errs
}

// mapKeysToSlice converts the set of keys of provided map to a slice of strings
func mapKeysToSlice(m map[PodNameRegexPrevious]sets.Set[string]) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k.PodNameRegex)
	}
	return keys
}

// filterContainerLogs compiles the provided podNameRegexStr and creates the map
// with Pod name as a key and slice of container names as a value. It iterates over the map and for each
// container name, it asynchronously (own Go routine) gets and filters the corresponding container
// log and the result is sent to the `recCh` channel.
func filterContainerLogs(ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	logRequest LogRequest,
	wg *sync.WaitGroup,
	recCh chan<- *recordWithError) {
	defer wg.Done()

	podToContainers, errs := createPodToContainersAndMessagesMapping(ctx, coreClient, logRequest)
	if len(errs) > 0 {
		errMessage := fmt.Sprintf("Failed to get matching pod names for namespace %s: %v\n", logRequest.Namespace, utils.UniqueErrors(errs))
		klog.Errorf(errMessage)
		for _, err := range errs {
			recCh <- &recordWithError{
				r:   nil,
				err: fmt.Errorf("failed to get matching pod names for namespace %s: %v", logRequest.Namespace, err),
			}
		}
		return
	}

	var wgContainers sync.WaitGroup
	for podName, containersAndMessages := range podToContainers {
		messagesRegex, err := listOfMessagesToRegex(containersAndMessages.messsages)
		if err != nil {
			errMessage := fmt.Sprintf("Failed to compile the list of messages for one of the %s Pod name regexes for %s namespace: %v",
				mapKeysToSlice(logRequest.PodNameRegexToMessages),
				logRequest.Namespace,
				err)
			recCh <- &recordWithError{
				r:   nil,
				err: errors.New(errMessage),
			}
			continue
		}
		wgContainers.Add(len(containersAndMessages.containerNames))
		for _, container := range containersAndMessages.containerNames {
			containerLogReq := ContainerLogRequest{
				Namespace:     logRequest.Namespace,
				ContainerName: container,
				PodName:       podName,
				MessageRegex:  messagesRegex,
				Previous:      containersAndMessages.previous,
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
// them with namespace name as the key and the logRequest as the value. The LogRequest data structure
// contains another map for mapping Pod name regex together with Previous value
// (saying previous container run) to list of messages.
func groupRawLogRequestsByNamespace(rawLogRequests []RawLogRequest) map[string]LogRequest {
	namespaceToLogRequestMap := make(map[string]LogRequest, len(rawLogRequests))
	for _, logRequest := range rawLogRequests {
		podNameRegexPrevious := PodNameRegexPrevious{PodNameRegex: logRequest.PodNameRegex, Previous: logRequest.Previous}
		existingLogRequest, ok := namespaceToLogRequestMap[logRequest.Namespace]

		if !ok {
			namespaceToLogRequestMap[logRequest.Namespace] = LogRequest{
				Namespace: logRequest.Namespace,
				PodNameRegexToMessages: map[PodNameRegexPrevious]sets.Set[string]{
					podNameRegexPrevious: sets.Set[string](sets.NewString(logRequest.Messages...)),
				},
			}
			continue
		}
		if setOfMessages, ok := existingLogRequest.PodNameRegexToMessages[podNameRegexPrevious]; ok {
			setOfMessages = setOfMessages.Union(sets.Set[string](sets.NewString(logRequest.Messages...)))
			existingLogRequest.PodNameRegexToMessages[podNameRegexPrevious] = setOfMessages
		} else {
			existingLogRequest.PodNameRegexToMessages[podNameRegexPrevious] = sets.Set[string](sets.NewString(logRequest.Messages...))
		}
		namespaceToLogRequestMap[logRequest.Namespace] = existingLogRequest
	}
	return namespaceToLogRequestMap
}

type containersAndMessages struct {
	containerNames []string
	messsages      sets.Set[string]
	previous       bool
}

// createPodToContainersAndMessagesMapping iterates over all the Pod name regular
// expression for the given logRequest and creates mapping between
// Pod name (matching the regular expression) and container names as well as messages
// required for the log filtering
func createPodToContainersAndMessagesMapping(ctx context.Context,
	coreCli corev1client.CoreV1Interface,
	logRequest LogRequest) (map[string]containersAndMessages, []error) {
	podContainers := make(map[string]containersAndMessages)
	podList, err := coreCli.Pods(logRequest.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var regexErrs []error
	for podNameRegexKey, messages := range logRequest.PodNameRegexToMessages {
		podNameRegex, err := regexp.Compile(podNameRegexKey.PodNameRegex)
		if err != nil {
			regexErrs = append(regexErrs, err)
			continue
		}
		for i := range podList.Items {
			pod := podList.Items[i]
			if podNameRegex.Match([]byte(pod.Name)) { // nolint: gocritic
				if cm, ok := podContainers[pod.Name]; ok {
					cm.messsages = cm.messsages.Union(messages)
					podContainers[pod.Name] = cm
				} else {
					var containerNames []string
					for i := range pod.Spec.Containers {
						c := pod.Spec.Containers[i]
						containerNames = append(containerNames, c.Name)
					}
					podContainers[pod.Name] = containersAndMessages{
						messsages:      messages,
						containerNames: containerNames,
						previous:       podNameRegexKey.Previous,
					}
				}
			}
		}
	}
	return podContainers, regexErrs
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
