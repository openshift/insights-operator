package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
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
	containersFilter := common.LogContainersFilter{
		Namespace:     "openshift-apiserver-operator",
		LabelSelector: "app=openshift-apiserver-operator",
	}
	messagesFilter := common.LogMessagesFilter{
		MessagesToSearch: []string{
			"the server has received too many requests and has asked us",
			"because serving request timed out and response had been started",
		},
		IsRegexSearch: false,
		SinceSeconds:  86400,     // last day
		LimitBytes:    1024 * 64, // maximum 64 kb of logs
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	return common.CollectLogsFromContainers(
		ctx,
		coreClient,
		containersFilter,
		messagesFilter,
		nil,
	)
}
