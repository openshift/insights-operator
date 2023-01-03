package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GatherSchedulerLogs Collects logs from pods in `openshift-kube-scheduler-namespace` from app
// `openshift-kube-scheduler` with following substring:
// - "PodTopologySpread"
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// None
//
// ### Location in archive
// | Version   | Path															  |
// | --------- | ---------------------------------------------------------------- |
// | >= 4.10   | config/pod/openshift-kube-scheduler/logs/{pod-name}/messages.log |
//
// ### Config ID
// `clusterconfig/scheduler_logs`
//
// ### Released version
// - 4.10
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherSchedulerLogs(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSchedulerLogs(ctx, gatherKubeClient.CoreV1())
}

func gatherSchedulerLogs(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	records, err := common.CollectLogsFromContainers(ctx, coreClient, common.LogContainersFilter{
		Namespace:     "openshift-kube-scheduler",
		LabelSelector: "app=openshift-kube-scheduler",
	}, common.LogMessagesFilter{
		MessagesToSearch: []string{"PodTopologySpread"},
		SinceSeconds:     logDefaultSinceSeconds,
		LimitBytes:       logDefaultLimitBytes,
	}, func(namespace string, podName string, containerName string) string {
		return fmt.Sprintf("config/pod/%s/logs/%s/messages.log", namespace, podName)
	})
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
