package clusterconfig

import (
	"bufio"
	"context"
	"fmt"
	"k8s.io/klog/v2"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/record"
)

// gatherLogsFromPodsInNamespace collects logs from all pods in provided namespace
//   - messagesToSearch are the messages to filter the logs(case-insensitive)
//   - sinceSeconds sets the moment to fetch logs from(current time - sinceSeconds)
//   - limitBytes sets the maximum amount of logs that can be fetched
//   - logFileName sets the name of the file to save logs to.
// Actual location is `config/pod/{namespace}/logs/{podName}/{fileName}.log`
func gatherLogsFromPodsInNamespace(
	g *Gatherer,
	namespace string,
	messagesToSearch []string,
	sinceSeconds int64,
	limitBytes int64,
	logFileName string,
) ([]record.Record, error) {
	ctx := g.ctx

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, err
	}

	coreClient := gatherKubeClient.CoreV1()

	pods, err := coreClient.Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var records []record.Record

	for _, pod := range pods.Items {
		request := coreClient.Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			SinceSeconds: &sinceSeconds,
			LimitBytes:   &limitBytes,
		})

		logs, err := filterLogs(ctx, request, messagesToSearch)
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

	if len(pods.Items) == 0 {
		klog.Infof("no pods in %v namespace were found", namespace)
	}

	return records, nil
}

func filterLogs(ctx context.Context, request *restclient.Request, messagesToSearch []string) (string, error) {
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
			if strings.Contains(strings.ToLower(line), strings.ToLower(messageToSearch)) {
				result += line + "\n"
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return result, nil
}
