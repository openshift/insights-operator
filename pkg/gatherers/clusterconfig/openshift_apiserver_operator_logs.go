package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenShiftAPIServerOperatorLogs collects logs from openshift-apiserver-operator with following substrings:
//   - "the server has received too many requests and has asked us"
//   - "because serving request timed out and response had been started"
//
// The Kubernetes API:
//       https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see:
//       https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/{namespace-name}/logs/{pod-name}/errors.log
func (g *Gatherer) GatherOpenShiftAPIServerOperatorLogs(ctx context.Context) ([]record.Record, []error) {

	containersFilter := logContainersFilter{
		namespace:     "openshift-apiserver-operator",
		labelSelector: "app=openshift-apiserver-operator",
	}
	messagesFilter := logMessagesFilter{
		messagesToSearch: []string{
			"the server has received too many requests and has asked us",
			"because serving request timed out and response had been started",
		},
		isRegexSearch: false,
		sinceSeconds:  86400,     // last day
		limitBytes:    1024 * 64, // maximum 64 kb of logs
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := gatherLogsFromContainers(
		ctx,
		coreClient,
		containersFilter,
		messagesFilter,
		"errors",
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
