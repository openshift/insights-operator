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
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

type logContainersFilter struct {
	namespace                string
	labelSelector            string
	containerNameRegexFilter string
}

type logMessagesFilter struct {
	messagesToSearch []string
	isRegexSearch    bool
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
//nolint: unparam
func gatherLogsFromContainers(
	ctx context.Context,
	coreClient v1.CoreV1Interface,
	containersFilter logContainersFilter,
	messagesFilter logMessagesFilter,
	logFileName string,
) ([]record.Record, error) {
	pods, err := coreClient.Pods(containersFilter.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: containersFilter.labelSelector,
	})
	if err != nil {
		return nil, err
	}

	var records []record.Record

	for i := range pods.Items {
		var containers []string
		for j := range pods.Items[i].Spec.Containers {
			containers = append(containers, pods.Items[i].Spec.Containers[j].Name)
		}
		for j := range pods.Items[i].Spec.InitContainers {
			containers = append(containers, pods.Items[i].Spec.InitContainers[j].Name)
		}

		for _, container := range containers {
			if len(containersFilter.containerNameRegexFilter) > 0 {
				match, err := regexp.MatchString(containersFilter.containerNameRegexFilter, container)
				if err != nil {
					return nil, err
				}
				if !match {
					continue
				}
			}

			request := coreClient.Pods(containersFilter.namespace).GetLogs(pods.Items[i].Name, &corev1.PodLogOptions{
				Container:    container,
				SinceSeconds: &messagesFilter.sinceSeconds,
				LimitBytes:   &messagesFilter.limitBytes,
			})

			logs, err := filterLogs(ctx, request, messagesFilter.messagesToSearch, messagesFilter.isRegexSearch)
			if err != nil {
				return nil, err
			}

			if len(strings.TrimSpace(logs)) != 0 {
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/pod/%s/logs/%s/%s.log", pods.Items[i].Namespace, pods.Items[i].Name, logFileName),
					Item: marshal.Raw{Str: logs},
				})
			}
		}
	}

	if len(pods.Items) == 0 {
		klog.Infof("no pods in %v namespace were found", containersFilter.namespace)
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
			} else if strings.Contains(strings.ToLower(line), strings.ToLower(messageToSearch)) {
				result += line + "\n"
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return result, nil
}
