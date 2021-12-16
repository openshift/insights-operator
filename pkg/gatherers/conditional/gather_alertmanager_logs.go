package conditional

import (
	"context"
	"fmt"

	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"

	"github.com/openshift/insights-operator/pkg/gatherers"
)

// BuildGatherAlertmanagerLogs collects alertmanager logs for pods firing one the configured alerts.
//
// * Location in archive: conditional/namespaces/<namespace>/pods/<pod>/containers/<container>/logs/last-{i}-lines.log
// * Id in config: alertmanager_logs
// * Since versions:
//   * 4.10+
func (g *Gatherer) BuildGatherAlertmanagerLogs(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherAlertmanagerLogsParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherAlertmanagerLogsParams{},
			paramsInterface)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}

			coreClient := kubeClient.CoreV1()

			records, errs := g.gatherAlertmanagerLogs(ctx, params, coreClient)
			if errs != nil {
				return records, errs
			}
			return records, nil
		},
	}, nil
}

func (g *Gatherer) gatherAlertmanagerLogs(
	ctx context.Context,
	params GatherAlertmanagerLogsParams,
	coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	alertInstances, ok := g.firingAlerts[params.AlertName]
	if !ok {
		return nil, []error{fmt.Errorf("conditional gatherer triggered, but specified alert %q is not firing", params.AlertName)}
	}

	var errs []error
	var records []record.Record

	for _, alertLabels := range alertInstances {
		alertPodNamespace, err := getAlertPodNamespace(alertLabels)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		alertPodName, err := getAlertPodName(alertLabels)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		logAlertmanager, err := common.CollectLogsFromContainers(
			ctx,
			coreClient,
			common.LogContainersFilter{
				Namespace:                alertPodNamespace,
				FieldSelector:            fmt.Sprintf("metadata.name=%s", alertPodName),
				ContainerNameRegexFilter: "^alertmanager$",
			},
			common.LogMessagesFilter{
				TailLines: params.TailLines,
			},
			func(namespace string, podName string, containerName string) string {
				return fmt.Sprintf(
					"%s/namespaces/%s/pods/%s/containers/%s/logs/last-%d-lines.log",
					g.GetName(),
					namespace,
					podName,
					containerName,
					params.TailLines,
				)
			},
		)
		if err != nil {
			newErr := fmt.Errorf("unable to get container logs: %v", err)
			klog.Warningln(newErr.Error())
			errs = append(errs, newErr)
		}

		records = append(records, logAlertmanager...)
	}

	return records, errs
}
