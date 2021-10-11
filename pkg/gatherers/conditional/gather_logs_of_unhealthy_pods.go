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

// BuildGatherLogsOfUnhealthyPods collects either current or previous logs for pods firing one of the configured alerts.
//
// * Location in archive: conditional/unhealthy_logs/<namespace>/<pod>/<container>/[current|previous].log
// * Since versions:
//   * 4.10+
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
			kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}
			records, errs := g.gatherLogsOfUnhealthyPods(ctx, kubeClient.CoreV1(), params)
			if len(errs) > 0 {
				return records, errs
			}
			return records, nil
		},
		CanFail: canConditionalGathererFail,
	}, nil
}

func (g *Gatherer) gatherLogsOfUnhealthyPods(
	ctx context.Context, coreClient v1.CoreV1Interface, params GatherLogsOfUnhealthyPodsParams,
) ([]record.Record, []error) {
	errs := []error{}
	records := []record.Record{}

	alertInstances, ok := g.firingAlerts[params.AlertName]
	if !ok {
		return nil, nil
	}
	for _, alertLabels := range alertInstances {
		alertNamespace, ok := alertLabels["namespace"]
		if !ok {
			newErr := fmt.Errorf("alert is missing 'namespace' label")
			klog.Warningln(newErr.Error())
			errs = append(errs, newErr)
			continue
		}
		alertPod, ok := alertLabels["pod"]
		if !ok {
			newErr := fmt.Errorf("alert is missing 'pod' label")
			klog.Warningln(newErr.Error())
			errs = append(errs, newErr)
			continue
		}
		// The container label may not be present for all alerts (e.g., KubePodNotReady).
		alertContainer := alertLabels["container"]

		containerFilter := ""
		if alertContainer != "" {
			containerFilter = fmt.Sprintf("^%s$", alertContainer)
		}

		logRecords, err := common.CollectLogsFromContainers(ctx, coreClient,
			common.LogContainersFilter{
				Namespace:                alertNamespace,
				FieldSelector:            fmt.Sprintf("metadata.name=%s", alertPod),
				ContainerNameRegexFilter: containerFilter,
			},
			common.LogMessagesFilter{
				TailLines: params.TailLines,
				Previous:  params.Previous,
			},
			func(namespace string, podName string, containerName string) string {
				logKind := "current"
				if params.Previous {
					logKind = "previous"
				}
				return fmt.Sprintf("%s/unhealthy_logs/%s/%s/%s/%s.log", g.GetName(), namespace, podName, containerName, logKind)
			})
		if err != nil {
			// This can happen when the pod is destroyed but the alert still exists.
			newErr := fmt.Errorf("unable to get container logs: %v", err)
			klog.Warningln(newErr.Error())
			errs = append(errs, newErr)
			continue
		}

		records = append(records, logRecords...)
	}

	return records, errs
}
