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
// * Location in archive: conditional/namespaces/<namespace>/pods/<pod>/containers/<container>/<logs|logs-previous>/last-<tail length>-lines.log
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
			return g.gatherLogsOfUnhealthyPods(ctx, kubeClient.CoreV1(), params)
		},
	}, nil
}

func (g *Gatherer) gatherLogsOfUnhealthyPods(
	ctx context.Context, coreClient v1.CoreV1Interface, params GatherLogsOfUnhealthyPodsParams,
) ([]record.Record, []error) {
	errs := []error{}
	records := []record.Record{}

	alertInstances, ok := g.firingAlerts[params.AlertName]
	if !ok {
		return nil, []error{fmt.Errorf("conditional gatherer triggered, but specified alert %q is not firing", params.AlertName)}
	}
	for _, alertLabels := range alertInstances {
		alertNamespace, err := getAlertPodNamespace(alertLabels)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		alertPod, err := getAlertPodName(alertLabels)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// The container label may not be present for all alerts (e.g., KubePodNotReady).
		containerFilter := ""
		if alertContainer, ok := alertLabels["container"]; ok && alertContainer != "" {
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
				logDirName := "logs"
				if params.Previous {
					logDirName = "logs-previous"
				}
				return fmt.Sprintf(
					"%s/namespaces/%s/pods/%s/containers/%s/%s/last-%d-lines.log",
					g.GetName(),
					namespace,
					podName,
					containerName,
					logDirName,
					params.TailLines,
				)
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
