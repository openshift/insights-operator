package conditional

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

func (g *Gatherer) BuildGatherLogsOfUnhealthyPods(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherLogsOfUnhealthyPodsParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherLogsOfUnhealthyPodsParams{}, paramsInterface,
		)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			records, err := g.gatherLogsOfUnhealthyPods(ctx, params)
			if err != nil {
				return records, []error{err}
			}
			return records, nil
		},
		CanFail: canConditionalGathererFail,
	}, nil
}

func (g *Gatherer) gatherLogsOfUnhealthyPods(
	ctx context.Context, params GatherLogsOfUnhealthyPodsParams,
) ([]record.Record, error) {
	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, err
	}
	coreClient := kubeClient.CoreV1()

	records := []record.Record{}
	records = append(records, getLogsForAlerts(g, ctx, coreClient, params.AlertsCurrent, params.TailLinesCurrent)...)
	records = append(records, getLogsForAlerts(g, ctx, coreClient, params.AlertsPrevious, params.TailLinesPrevious)...)

	return records, nil
}

func getLogsForAlerts(g *Gatherer, ctx context.Context, coreClient v1.CoreV1Interface, alertNames []string, tailLines int64) []record.Record {
	records := []record.Record{}

	for _, alertName := range alertNames {
		alertInstances, ok := g.firingAlerts[alertName]
		if !ok {
			continue
		}
		for _, alertLabels := range alertInstances {
			alertNamespace, ok := alertLabels["namespace"]
			if !ok {
				klog.Warningf("alert is missing 'namespace' label")
				continue
			}
			alertPod, ok := alertLabels["pod"]
			if !ok {
				klog.Warningf("alert is missing 'pod' label")
				continue
			}
			alertContainer, ok := alertLabels["container"]
			if !ok {
				klog.Warningf("alert is missing 'container' label")
				continue
			}

			alertRecords, err := common.CollectLogsFromContainers(ctx, coreClient,
				common.LogContainersFilter{
					Namespace:                alertNamespace,
					LabelSelector:            fmt.Sprintf("pod=%s", alertPod),
					ContainerNameRegexFilter: fmt.Sprintf("^%s$", alertContainer),
				},
				common.LogMessagesFilter{
					TailLines: tailLines,
				},
				nil)
			if err != nil {
				// This can happen when the pod is destroyed but the alert still exists.
				klog.Warningf("unable to get container logs: %v", err)
				continue
			}

			records = append(records, alertRecords...)
		}
	}

	return records
}
