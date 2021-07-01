package conditional

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// BuildGatherLogsOfNamespace creates a gathering closure which collects logs from pods in the provided namespace
// Params is of type GatherLogsOfNamespaceParams:
//   - namespace string - namespace from which to collect logs
//   - tail_lines int64 - a number of log lines to keep for each container
//
// The Kubernetes API:
//          https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see:
//          https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: conditional/namespaces/{namespace}/pods/{pod_name}/containers/{container_name}/logs/last-{n}-lines.log
// * Since versions:
//   * 4.9+
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
			records, err := g.gatherLogsOfNamespace(
				ctx,
				params.Namespace,
				params.TailLines,
			)
			if err != nil {
				return records, []error{err}
			}
			return records, nil
		},
		CanFail: canConditionalGathererFail,
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
			Namespace: namespace,
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
