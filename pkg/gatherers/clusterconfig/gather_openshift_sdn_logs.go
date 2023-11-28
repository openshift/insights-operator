// nolint: dupl
package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenshiftSDNLogs Collects logs from pods in `openshift-sdn` namespace with following substrings:
// - "Got OnEndpointsUpdate for unknown Endpoints",
// - "Got OnEndpointsDelete for unknown Endpoints",
// - "Unable to update proxy firewall for policy",
// - "Failed to update proxy firewall for policy",
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// - docs/insights-archive-sample/config/pod/openshift-sdn/logs/sdn-f2694/errors.log
//
// ### Location in archive
// - `config/pod/openshift-sdn/logs/{name}/errors.log`
//
// ### Config ID
// `clusterconfig/openshift_sdn_logs`
//
// ### Released version
// - 4.7.0
//
// ### Backported versions
// - 4.6.19+
//
// ### Changes
// None
func (g *Gatherer) GatherOpenshiftSDNLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := &common.LogResourceFilter{
		Namespace:     "openshift-sdn",
		LabelSelector: "app=sdn",
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
		getGatherOpenshiftSDNLogsMessageFilter(),
		nil,
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}

func getGatherOpenshiftSDNLogsMessageFilter() *common.LogMessagesFilter {
	return &common.LogMessagesFilter{
		MessagesToSearch: []string{
			"Got OnEndpointsUpdate for unknown Endpoints",
			"Got OnEndpointsDelete for unknown Endpoints",
			"Unable to update proxy firewall for policy",
			"Failed to update proxy firewall for policy",
		},
		IsRegexSearch: false,
		SinceSeconds:  logDefaultSinceSeconds,
		LimitBytes:    logDefaultLimitBytes,
	}
}
