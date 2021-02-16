package clusterconfig

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
)

type logsContainersFilter struct {
	namespace                string
	labelSelector            string
	containerNameRegexFilter string
}

type logMessagesFilter struct {
	messagesToSearch []string
	regexSearch      bool
	sinceSeconds     int64
	limitBytes       int64
}

// gatherLogsFromContainers collects logs from containers
//   - containerFilter allows you to specify
//     - namespace in which to search for pods
//     - labelSelector to filter pods by their labels (keep empty to not filter)
//     - containerNameRegexFilter to filter containers in the pod (keep empty to not filter)
//   - logMessagesFilter allows you to specify
//     - messagesToSearch to filter the logs by substrings (case-insensitive)
//       or regex (add `(?i)` in the beginning to make search case-insensitive)
//     - regexSearch which makes messagesToSearch regex patterns, so you can accomplish more complicated search
//     - sinceSeconds which sets the moment to fetch the logs from (current time - sinceSeconds)
//     - limitBytes which sets the maximum amount of logs that can be fetched
//   - logFileName sets the name of the file to save the logs to.
//
// Location of the logs is `config/pod/{namespace}/logs/{podName}/{fileName}.log`
func gatherLogsFromContainers(
	ctx context.Context,
	coreClient v1.CoreV1Interface,
	containersFilter logsContainersFilter,
	logMessagesFilter logMessagesFilter,
	logFileName string,
) ([]record.Record, error) {
	var (
		namespace                = containersFilter.namespace
		labelSelector            = containersFilter.labelSelector
		containerNameRegexFilter = containersFilter.containerNameRegexFilter
		messagesToSearch         = logMessagesFilter.messagesToSearch
		regexSearch              = logMessagesFilter.regexSearch
		sinceSeconds             = logMessagesFilter.sinceSeconds
		limitBytes               = logMessagesFilter.limitBytes
	)

	pods, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	var records []record.Record

	for _, pod := range pods.Items {
		var containers []string
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}
		for _, container := range pod.Spec.InitContainers {
			containers = append(containers, container.Name)
		}

		for _, container := range containers {
			if len(containerNameRegexFilter) > 0 {
				match, err := regexp.MatchString(containerNameRegexFilter, container)
				if err != nil {
					return nil, err
				}
				if !match {
					continue
				}
			}

			request := coreClient.Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container:    container,
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
