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
	"k8s.io/klog"

	"github.com/openshift/insights-operator/pkg/record"
)

// gatherLogsFromPodsInNamespace collects logs from the pods in provided namespace
//   - messagesToSearch are the messages to filter the logs(case-insensitive)
//   - sinceSeconds sets the moment to fetch logs from(current time - sinceSeconds)
//   - limitBytes sets the maximum amount of logs that can be fetched
//   - logFileName sets the name of the file to save logs to.
//   - labelSelector allows you to filter pods by their labels
//   - regexSearch makes messagesToSearch regex patterns, so you can accomplish more complicated search
//
// Location of the logs is `config/pod/{namespace}/logs/{podName}/{fileName}.log`
func gatherLogsFromPodsInNamespace(
	ctx context.Context,
	coreClient v1.CoreV1Interface,
	namespace string,
	messagesToSearch []string,
	sinceSeconds int64,
	limitBytes int64,
	logFileName string,
	labelSelector string,
	regexSearch bool,
) ([]record.Record, error) {
	pods, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	var records []record.Record

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			request := coreClient.Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container:    container.Name,
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
