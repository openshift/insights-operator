package clusterconfig

import (
	"context"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/record"
)

func (g *Gatherer) GatherAlertmanagerLogs(ctx context.Context) ([]record.Record, []error) {
	// namespace: openshift-monitoring
	// kind: Alertmanager
	// ➜ oc -n openshift-monitoring get alertmanager -o yaml

	containersFilter := common.LogContainersFilter{
		Namespace:     "openshift-monitoring",
		LabelSelector: "app=alertmanager",
	}
	messagesFilter := common.LogMessagesFilter{
		SinceSeconds: 86400,     // last day
		LimitBytes:   1024 * 64, // maximum 64 kb of logs
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := common.CollectLogsFromContainers(
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
