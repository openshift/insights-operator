package common

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// LogContainersFilter allows you to filter containers
type LogContainersFilter struct {
	Namespace                string
	LabelSelector            string
	FieldSelector            string
	ContainerNameRegexFilter string
	MaxNamespaceContainers   int
}

// LogMessagesFilter allows you to filter messages
type LogMessagesFilter struct {
	MessagesToSearch []string
	IsRegexSearch    bool
	SinceSeconds     int64
	LimitBytes       int64
	TailLines        int64
	Previous         bool
}

// CollectLogsFromContainers collects logs from containers
//   - containerFilter allows you to specify
//   - namespace in which to search for pods
//   - labelSelector to filter pods by their labels (keep empty to not filter)
//   - containerNameRegexFilter to filter containers in the pod (keep empty to not filter)
//   - maxNamespaceContainers to limit the containers in the given namespace (keep empty to not limit)
//   - logMessagesFilter allows you to specify
//   - messagesToSearch to filter the logs by substrings (case-insensitive)
//     or regex (add `(?i)` in the beginning to make search case-insensitive). Leave nil to not filter.
//   - regexSearch which makes messagesToSearch regex patterns, so you can accomplish more complicated search
//   - sinceSeconds which sets the moment to fetch the logs from (current time - sinceSeconds)
//   - limitBytes which sets the maximum amount of logs that can be fetched
//   - tailLines which sets the maximum amount of log lines from the end that should be fetched
//   - buildLogFileName is the function returning filename for the current log,
//     if nil, the default implementation is used
//
// Default location of the logs is `config/pod/{namespace}/logs/{podName}/errors.log`,
//
//	you can override it with buildLogFileName
func CollectLogsFromContainers( //nolint:gocyclo
	ctx context.Context,
	coreClient v1.CoreV1Interface,
	containersFilter LogContainersFilter,
	messagesFilter LogMessagesFilter,
	buildLogFileName func(namespace string, podName string, containerName string) string,
) ([]record.Record, error) {
	if buildLogFileName == nil {
		buildLogFileName = func(namespace string, podName string, _ string) string {
			return fmt.Sprintf("config/pod/%s/logs/%s/errors.log", namespace, podName)
		}
	}

	pods, err := coreClient.Pods(containersFilter.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: containersFilter.LabelSelector,
		FieldSelector: containersFilter.FieldSelector,
	})
	if err != nil {
		return nil, err
	}

	var skippedContainers int
	var records []record.Record

	for i := range pods.Items {
		var containerNames []string
		for j := range pods.Items[i].Spec.Containers {
			containerNames = append(containerNames, pods.Items[i].Spec.Containers[j].Name)
		}
		for j := range pods.Items[i].Spec.InitContainers {
			containerNames = append(containerNames, pods.Items[i].Spec.InitContainers[j].Name)
		}

		containersLimited := containersFilter.MaxNamespaceContainers > 0 && len(records) >= containersFilter.MaxNamespaceContainers
		if containersLimited {
			skippedContainers += len(containerNames)
			continue
		}

		pod := &pods.Items[i]

		for _, containerName := range containerNames {
			if len(containersFilter.ContainerNameRegexFilter) > 0 {
				match, err := regexp.MatchString(containersFilter.ContainerNameRegexFilter, containerName)
				if err != nil {
					return nil, err
				}
				if !match {
					continue
				}
			}

			if containersLimited {
				skippedContainers = len(containerNames) - containersFilter.MaxNamespaceContainers
				break
			}

			request := coreClient.Pods(pod.Namespace).GetLogs(pod.Name, podLogOptions(containerName, messagesFilter))

			logs, err := filterLogs(ctx, request, messagesFilter.MessagesToSearch, messagesFilter.IsRegexSearch)
			if err != nil {
				return nil, err
			}

			if len(strings.TrimSpace(logs)) != 0 {
				records = append(records, record.Record{
					Name: buildLogFileName(pod.Namespace, pod.Name, containerName),
					Item: marshal.Raw{Str: logs},
				})
			}
		}
	}

	if len(pods.Items) == 0 {
		klog.Infof("no pods in %v namespace were found", containersFilter.Namespace)
	}

	if skippedContainers > 0 {
		return records, fmt.Errorf("skipping %d containers on namespace %s (max: %d)",
			skippedContainers, containersFilter.Namespace, containersFilter.MaxNamespaceContainers)
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
	return FilterLogFromScanner(scanner, messagesToSearch, regexSearch, nil)
}

// FilterLogFromScanner filters the desired messages from the log
func FilterLogFromScanner(scanner *bufio.Scanner, messagesToSearch []string, regexSearch bool,
	cb func(lines []string) []string) (string, error) {
	var result []string

	var messagesRegexp *regexp.Regexp
	if regexSearch {
		messagesRegexp = regexp.MustCompile(strings.Join(messagesToSearch, "|"))
	}

	for scanner.Scan() {
		line := scanner.Text()
		if len(messagesToSearch) == 0 {
			result = append(result, line)
			continue
		}

		if regexSearch && messagesRegexp != nil {
			matches := messagesRegexp.MatchString(line)
			if matches {
				result = append(result, line)
			}
		} else {
			for _, messageToSearch := range messagesToSearch {
				if strings.Contains(strings.ToLower(line), strings.ToLower(messageToSearch)) {
					result = append(result, line)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if cb != nil {
		result = cb(result)
	}

	return strings.Join(result, "\n"), nil
}

func podLogOptions(containerName string, messagesFilter LogMessagesFilter) *corev1.PodLogOptions {
	sinceSeconds := &messagesFilter.SinceSeconds
	if messagesFilter.SinceSeconds == 0 {
		sinceSeconds = nil
	}

	limitBytes := &messagesFilter.LimitBytes
	if messagesFilter.LimitBytes == 0 {
		limitBytes = nil
	}

	tailLines := &messagesFilter.TailLines
	if messagesFilter.TailLines == 0 {
		tailLines = nil
	}

	return &corev1.PodLogOptions{
		Container:    containerName,
		SinceSeconds: sinceSeconds,
		LimitBytes:   limitBytes,
		TailLines:    tailLines,
		Previous:     messagesFilter.Previous,
		Timestamps:   true,
	}
}
