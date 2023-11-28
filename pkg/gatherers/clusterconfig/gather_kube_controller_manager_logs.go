package clusterconfig

// nolint: dupl

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherKubeControllerManagerLogs Collects logs from `kube-controller-manager` pods in the
// `openshift-kube-controller-manager` namespace with following substrings:
// - "Internal error occurred: error resolving resource",
// - "syncing garbage collector with updated resources from discovery",
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.10/rest_api/workloads_apis/pod-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// - docs/insights-archive-sample/config/pod/openshift-kube-controller-manager/logs/kube-controller-manager-ip-10-0-168-11.us-east-2.compute.internal/errors.log
//
// ### Location in archive
// - `config/pod/openshift-kube-controller-manager/logs/{pod-name}/errors.log`
//
// ### Config ID
// `clusterconfig/kube_controller_manager_logs`
//
// ### Released version
// - 4.11.0
//
// ### Backported versions
// - 4.10.6+
// - 4.9.27+
//
// ### Changes
// None
func (g *Gatherer) GatherKubeControllerManagerLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := &common.LogResourceFilter{
		Namespace:                "openshift-kube-controller-manager",
		LabelSelector:            "app=kube-controller-manager",
		ContainerNameRegexFilter: "kube-controller-manager",
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
		getKubeControllerManagerLogsMessagesFilter(),
		nil,
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}

func getKubeControllerManagerLogsMessagesFilter() *common.LogMessagesFilter {
	return &common.LogMessagesFilter{
		MessagesToSearch: []string{
			"Internal error occurred: error resolving resource",
			"syncing garbage collector with updated resources from discovery",
		},
		IsRegexSearch: true,
		SinceSeconds:  logDefaultSinceSeconds,
		LimitBytes:    5 * logDefaultLimitBytes,
	}
}
