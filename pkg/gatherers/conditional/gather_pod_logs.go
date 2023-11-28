package conditional

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"

	"github.com/openshift/insights-operator/pkg/gatherers"
)

func (g *Gatherer) BuildGatherPodLogs(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherPodLogsParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherPodLogsParams{}, paramsInterface,
		)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}
			coreClient := kubeClient.CoreV1()
			return g.gatherPodLogs(ctx, &params, coreClient)
		},
	}, nil
}

func (g *Gatherer) gatherPodLogs(ctx context.Context, params *GatherPodLogsParams,
	coreClient v1.CoreV1Interface) ([]record.Record, []error) {
	records, err := common.CollectLogsFromContainers(
		ctx,
		coreClient,
		&params.ResourceFilter,
		&params.LogMessageFilter,
		func(namespace string, podName string, containerName string) string {
			filename := "current.log"
			if params.LogMessageFilter.Previous {
				filename = "previous.log"
			}
			return fmt.Sprintf(
				"%v/pod_logs/%v/%v/%v/%v",
				g.GetName(), namespace, podName, containerName, filename,
			)
		},
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
