package conditional

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// BuildGatherContainersLogs collects either current or previous containers logs for pods firing one of the configured alerts.
//
// * Location in archive: conditional/namespaces/{namespace}/pods/{pod}/containers/{container}/{logs|logs-previous}/last-{tail-length}-lines.log
// * Id in config: conditional/containers_logs
// * Since versions:
//   * 4.10+
func (g *Gatherer) BuildGatherContainersLogs(paramsInterface interface{}) (gatherers.GatheringClosure, error) { // nolint: dupl
	params, ok := paramsInterface.(GatherContainersLogsParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherContainersLogsParams{},
			paramsInterface)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
			if err != nil {
				return nil, []error{err}
			}
			coreClient := kubeClient.CoreV1()
			return g.gatherContainersLogs(ctx, params, coreClient)
		},
	}, nil
}

func (g *Gatherer) gatherContainersLogs(
	ctx context.Context,
	params GatherContainersLogsParams,
	coreClient corev1client.CoreV1Interface,
) ([]record.Record, []error) {
	alertInstances, ok := g.firingAlerts[params.AlertName]
	if !ok {
		err := fmt.Errorf("conditional gather triggered, but specified alert %q is not firing", params.AlertName)
		return nil, []error{err}
	}

	const logMissingAlert = "%s at alertName: %s"

	var errs []error
	var records []record.Record

	for _, alertLabels := range alertInstances {
		podNamespace, err := getAlertPodNamespace(alertLabels)
		if err != nil {
			klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
			errs = append(errs, err)
			continue
		}
		podName, err := getAlertPodName(alertLabels)
		if err != nil {
			klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
			errs = append(errs, err)
			continue
		}
		var podContainer string
		if len(params.Container) > 0 {
			podContainer = params.Container
		} else {
			podContainer, err = getAlertPodContainer(alertLabels)
			if err != nil {
				klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
				errs = append(errs, err)
			}
		}

		logContainersFilter := common.LogContainersFilter{
			Namespace:     podNamespace,
			FieldSelector: fmt.Sprintf("metadata.name=%s", podName),
		}

		// The container label may not be present for all alerts (e.g., KubePodNotReady).
		if len(podContainer) > 0 {
			logContainersFilter.ContainerNameRegexFilter = fmt.Sprintf("^%s$", podContainer)
		}

		logRecords, err := common.CollectLogsFromContainers(
			ctx,
			coreClient,
			logContainersFilter,
			common.LogMessagesFilter{
				TailLines: params.TailLines,
				Previous:  params.Previous,
			},
			func(namespace, podName, containerName string) string {
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
			},
		)
		if err != nil {
			newErr := fmt.Errorf("unable to get container logs: %v", err)
			klog.Warning(newErr.Error())
			errs = append(errs, newErr)
		}

		records = append(records, logRecords...)
	}

	return records, errs
}
