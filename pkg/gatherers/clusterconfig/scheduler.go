package clusterconfig

import (
	"context"
	"fmt"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSchedulers collects information about schedulers
//
// The API:
//         https://docs.openshift.com/container-platform/4.9/rest_api/config_apis/scheduler-config-openshift-io-v1.html
//
// * Location in archive: config/schedulers/cluster.json
// * Since versions:
//   * 4.10+
func (g *Gatherer) GatherSchedulers(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSchedulerInfo(ctx, gatherConfigClient)
}

func gatherSchedulerInfo(
	ctx context.Context, configClient configv1client.ConfigV1Interface,
) ([]record.Record, []error) {
	schedulers, err := configClient.Schedulers().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i := range schedulers.Items {
		scheduler := &schedulers.Items[i]

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/schedulers/%v", scheduler.Name),
			Item: record.ResourceMarshaller{Resource: scheduler},
		})
	}

	return records, nil
}

// GatherSchedulerLogs collects logs from pods in openshift-kube-scheduler-namespace from app openshift-kube-scheduler
// with following substring:
//   - "PodTopologySpread"
//
// The Kubernetes API:
//         https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see:
//         https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/openshift-kube-scheduler/logs/{pod-name}/messages.log
// * Id in config: clusterconfig/scheduler_logs
// * Since versions:
//   * 4.10+
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
