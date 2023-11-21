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

// LogResourceFilter allows you to filter containers
type LogResourceFilter struct {
	Namespace                string `json:"namespace"`
	LabelSelector            string `json:"label_selector,omitempty"`
	FieldSelector            string `json:"field_selector,omitempty"`
	ContainerNameRegexFilter string `json:"container_name_regex_filter,omitempty"`
	PodNameRegexFilter       string `json:"pod_name_regex_filter,omitempty"`
	MaxNamespaceContainers   int    `json:"max_namespace_containers,omitempty"`
}

// LogMessagesFilter allows you to filter messages
type LogMessagesFilter struct {
	MessagesToSearch []string `json:"messages_to_search,omitempty"`
	IsRegexSearch    bool     `json:"is_regex_search,omitempty"`
	SinceSeconds     int64    `json:"since_seconds,omitempty"`
	LimitBytes       int64    `json:"limit_bytes,omitempty"`
	TailLines        int64    `json:"tail_lines,omitempty"`
	Previous         bool     `json:"previous,omitempty"`
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
	containersFilter LogResourceFilter,
	messagesFilter LogMessagesFilter,
	buildLogFileName func(namespace, podName, containerName string) string,
) ([]record.Record, error) {
	if buildLogFileName == nil {
		buildLogFileName = func(namespace, podName, containerName string) string {
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

	var podNameRegex *regexp.Regexp
	if len(containersFilter.PodNameRegexFilter) > 0 {
		podNameRegex = regexp.MustCompile(containersFilter.PodNameRegexFilter)
	}

	var messagesRegexp *regexp.Regexp
	if messagesFilter.IsRegexSearch {
		messagesRegexp = regexp.MustCompile(strings.Join(messagesFilter.MessagesToSearch, "|"))
	}

	for i := range pods.Items {
		pod := &pods.Items[i]

		if podNameRegex != nil {
			if !podNameRegex.MatchString(pod.Name) {
				continue
			}
		}

		containerNames := podContainers(pod)

		containersLimited := containersFilter.MaxNamespaceContainers > 0 && len(records) >= containersFilter.MaxNamespaceContainers
		if containersLimited {
			skippedContainers += len(containerNames)
			continue
		}

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

			request := coreClient.Pods(containersFilter.Namespace).GetLogs(pod.Name, podLogOptions(containerName, messagesFilter))

			logs, err := filterLogs(ctx, request, messagesFilter.MessagesToSearch, messagesRegexp)
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

func podContainers(pod *corev1.Pod) []string {
	var containerNames []string
	for j := range pod.Spec.Containers {
		containerNames = append(containerNames, pod.Spec.Containers[j].Name)
	}
	for j := range pod.Spec.InitContainers {
		containerNames = append(containerNames, pod.Spec.InitContainers[j].Name)
	}
	return containerNames
}

func filterLogs(
	ctx context.Context, request *restclient.Request, messagesToSearch []string, messagesRegexp *regexp.Regexp,
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
	return FilterLogFromScanner(scanner, messagesToSearch, messagesRegexp, nil)
}

// FilterLogFromScanner filters the desired messages from the log
func FilterLogFromScanner(scanner *bufio.Scanner, messagesToSearch []string, messagesRegexp *regexp.Regexp,
	cb func(lines []string) []string) (string, error) {
	var result []string

	for scanner.Scan() {
		line := scanner.Text()

		if messagesRegexp != nil {
			matches := messagesRegexp.MatchString(line)
			if matches {
				result = append(result, line)
			}
			continue
		}

		for _, messageToSearch := range messagesToSearch {
			if strings.Contains(strings.ToLower(line), strings.ToLower(messageToSearch)) {
				result = append(result, line)
			}
		}

		if len(messagesToSearch) == 0 {
			result = append(result, line)
			continue
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
