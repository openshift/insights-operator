package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// Extraction of dependency allows testing
var ingressCollectLogsFromContainers = common.CollectLogsFromContainers

// GatherOpenShiftIngressLogs collects logs from openshift-ingress:
//   - if the log line is on error level
//   - if the log line is on warning level and contains "error" substring
//
// The Kubernetes API:
//
//	https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
//
// Response see:
//
//	https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/openshift-ingress/logs/{pod-name}/errors.log
// * Id in config: cluspoterconfig/openshift_ingress_logs
// * Since version:
//   - 4.12+
func (g *Gatherer) GatherOpenShiftIngressLogs(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	coreClient := gatherKubeClient.CoreV1()

	containersFilter := common.LogContainersFilter{
		Namespace: "openshift-ingress",
	}

	messagesFilter := common.LogMessagesFilter{
		MessagesToSearch: []string{
			"E\\d+\\s",
			"W\\d+\\s.*error.*",
		},
		SinceSeconds:  logDefaultSinceSeconds,
		LimitBytes:    logDefaultLimitBytes,
		IsRegexSearch: true,
	}

	records, err := ingressCollectLogsFromContainers(
		ctx,
		coreClient,
		containersFilter,
		messagesFilter,
		nil,
	)

	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
