//nolint: dupl
package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherKubeControllerManagerLogs collects logs from kube-controller-manager pods in the openshift-kube-controller-manager namespace with following substrings:
//   - "Internal error occurred: error resolving resource",
//   - "syncing garbage collector with updated resources from discovery",
//
// The Kubernetes API:
//          https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see:
//          https://docs.openshift.com/container-platform/4.10/rest_api/workloads_apis/pod-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/openshift-kube-controller-manager/logs/{pod-name}/errors.log
// * Since versions:
//   * 4.9.27
//   * 4.10.6
//   * 4.11+
func (g *Gatherer) GatherKubeControllerManagerLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := common.LogContainersFilter{
		Namespace:                "openshift-kube-controller-manager",
		LabelSelector:            "app=kube-controller-manager",
		ContainerNameRegexFilter: "kube-controller-manager",
	}
	messagesFilter := common.LogMessagesFilter{
		MessagesToSearch: []string{
			"Internal error occurred: error resolving resource",
			"syncing garbage collector with updated resources from discovery",
		},
		IsRegexSearch: true,
		SinceSeconds:  logDefaultSinceSeconds,
		LimitBytes:    0,
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := common.CollectLogsFromContainers(
		ctx,
		coreClient,
		containersFilter,
		messagesFilter,
		nil,
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
