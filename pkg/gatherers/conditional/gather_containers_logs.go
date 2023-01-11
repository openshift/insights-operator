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

// BuildGatherContainersLogs Collects either current or previous containers logs for pods firing one of the
// alerts from the conditions fetched from insights conditions service.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/conditional/namespaces/openshift-cluster-samples-operator/pods/cluster-samples-operator-8ffb9b45f-49mjr/containers/cluster-samples-operator-watch/logs/last-100-lines.log
//
// ### Location in archive
// - `conditional/namespaces/{namespace}/pods/{pod}/containers/{container}/{logs|logs-previous}/last-{tail-length}-lines.log`
//
// ### Config ID
// `conditional/containers_logs`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// None
//
// ### Changes
// None
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

type podInfo struct {
	name      string
	namespace string
	container string
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
		info, err := parseAlertLabels(alertLabels)
		if err != nil {
			klog.Warningf(logMissingAlert, err.Error(), params.AlertName)
			errs = append(errs, err)
		}
		if len(info.namespace) == 0 {
			continue
		}

		logContainersFilter := &common.LogContainersFilter{
			Namespace: info.namespace,
		}

		if len(params.Container) > 0 {
			logContainersFilter.ContainerNameRegexFilter = fmt.Sprintf("^%s$", params.Container)
		} else if len(info.container) > 0 {
			logContainersFilter.ContainerNameRegexFilter = fmt.Sprintf("^%s$", info.container)
		}

		if len(params.PodName) > 0 {
			logContainersFilter.PodNameRegexFilter = fmt.Sprintf("^%s$", params.PodName)
		} else {
			logContainersFilter.FieldSelector = fmt.Sprintf("metadata.name=%s", info.name)
		}

		logRecords, err := common.CollectLogsFromContainers(
			ctx,
			coreClient,
			logContainersFilter,
			&common.LogMessagesFilter{
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

func parseAlertLabels(labels AlertLabels) (podInfo, error) {
	var info podInfo
	podNamespace, err := getAlertPodNamespace(labels)
	if err != nil {
		return info, err
	}
	info.namespace = podNamespace

	podName, err := getAlertPodName(labels)
	if err != nil {
		return info, err
	}
	info.name = podName

	podContainer, err := getAlertPodContainer(labels)
	if err != nil {
		return info, err
	}
	info.container = podContainer

	return info, nil
}
