//nolint: dupl
package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenshiftSDNLogs collects logs from pods in openshift-sdn namespace with following substrings:
//   - "Got OnEndpointsUpdate for unknown Endpoints",
//   - "Got OnEndpointsDelete for unknown Endpoints",
//   - "Unable to update proxy firewall for policy",
//   - "Failed to update proxy firewall for policy",
//
// The Kubernetes API:
//          https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see:
//          https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/openshift-sdn/logs/{pod-name}/errors.log
// * Since versions:
//   * 4.6.19+
//   * 4.7+
func (g *Gatherer) GatherOpenshiftSDNLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := common.LogContainersFilter{
		Namespace:     "openshift-sdn",
		LabelSelector: "app=sdn",
	}
	messagesFilter := common.LogMessagesFilter{
		MessagesToSearch: []string{
			"Got OnEndpointsUpdate for unknown Endpoints",
			"Got OnEndpointsDelete for unknown Endpoints",
			"Unable to update proxy firewall for policy",
			"Failed to update proxy firewall for policy",
		},
		IsRegexSearch: false,
		SinceSeconds:  86400,
		LimitBytes:    1024 * 64,
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
