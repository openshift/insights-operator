package conditional

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// BuildGatherLogsOfNamespace Creates a gathering closure which collects logs from pods in the provided namespace.
//
// Params is of type GatherLogsOfNamespaceParams:
// - namespace string - namespace from which to collect logs
// - tail_lines int64 - a number of log lines to keep for each container
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// - docs/insights-archive-sample/config/.json
//
// ### Location in archive
// | Version   | Path																									|
// | --------- | ------------------------------------------------------------------------------------------------------ |
// | >= 4.9    | conditional/namespaces/{namespace}/pods/{pod_name}/containers/{container_name}/logs/last-{n}-lines.log |
//
// ### Config ID
// `conditional/logs_of_namespace`
//
// ### Released version
// - 4.9.0
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) BuildGatherLogsOfNamespace(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherLogsOfNamespaceParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherLogsOfNamespaceParams{}, paramsInterface,
		)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			records, err := g.gatherLogsOfNamespace(ctx, params.Namespace, params.TailLines)
			if err != nil {
				return records, []error{err}
			}
			return records, nil
		},
	}, nil
}

func (g *Gatherer) gatherLogsOfNamespace(ctx context.Context, namespace string, tailLines int64) ([]record.Record, error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, err
	}

	coreClient := kubeClient.CoreV1()

	fileName := fmt.Sprintf("last-%v-lines.log", tailLines)

	records, err := common.CollectLogsFromContainers(
		ctx,
		coreClient,
		common.LogContainersFilter{
			Namespace:              namespace,
			MaxNamespaceContainers: 64, // arbitrary fixed value
		},
		common.LogMessagesFilter{
			TailLines: tailLines,
		},
		func(namespace string, podName string, containerName string) string {
			return fmt.Sprintf(
				"%v/namespaces/%v/pods/%v/containers/%v/logs/%v",
				g.GetName(), namespace, podName, containerName, fileName,
			)
		},
	)
	if err != nil {
		return nil, err
	}

	return records, nil
}
