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

// This https://github.com/openshift/insights-operator/pull/675 will look like:
//type ResourceFilter struct {
//	Namespace     string `json:"namespace"`
//	LabelSelector string `json:"label_selector,omitempty"`
//	FieldSelector string `json:"field_selector,omitempty"`
//	ContainerName string `json:"container_name,omitempty"`
//	PodName       string `json:"pod_name,omitempty"`
//}
//type LogFilter struct {
//	MessagePatterns []string `json:"message_patterns,omitempty"`
//	TailLines       int64    `json:"tail_lines,omitempty"`
//	Previous        bool     `json:"previous,omitempty"`
//}

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
			return g.gatherPodLogs(ctx, params, coreClient)
		},
	}, nil
}

func (g *Gatherer) gatherPodLogs(ctx context.Context, params GatherPodLogsParams, coreClient v1.CoreV1Interface) ([]record.Record, []error) {
	// TODO filter namespace, it only allows `openshift-*` and `kubernetes-*`
	records, err := common.CollectLogsFromContainers(
		ctx,
		coreClient,
		params.ResourceFilter,
		params.LogMessageFilter,
		func(namespace string, podName string, containerName string) string {
			return fmt.Sprintf(
				"%v/logs/%v/%v/%v.log",
				g.GetName(), namespace, podName, containerName,
			)
		},
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
