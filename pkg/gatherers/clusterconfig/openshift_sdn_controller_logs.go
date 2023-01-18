// nolint: dupl
package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenshiftSDNControllerLogs Collects logs from sdn-controller pod in openshift-sdn namespace with
// following substrings:
//
// - "Node %s is not Ready": A node has been set offline for egress IPs because it is reported not ready at API
// - "Node %s may be offline... retrying": An egress node has failed the egress IP health check once,
// so it has big chances to be marked as offline soon or, at the very least, there has been a connectivity glitch.
// - "Node %s is offline": An egress node has failed enough probes to have been marked offline for egress IPs.
// If it has egress CIDRs assigned, its egress IPs have been moved to other nodes. Indicate issues at either the node
// or the network between the master and the node.
// - "Node %s is back online": This indicates that a node has recovered from the condition described
// at the previous message, by starting succeeding the egress IP health checks. Useful just in case that previous
// “Node %s is offline” messages are lost, so that we have a clue that there was failure previously.
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// - docs/insights-archive-sample/config/pod/openshift-sdn/logs/sdn-f2694/errors.log
// - docs/insights-archive-sample/config/pod/openshift-sdn/logs/sdn-controller-l8gq9/errors.log
//
// ### Location in archive
// - `config/pod/openshift-sdn/logs/{pod-name}/errors.log`
//
// ### Config ID
// `clusterconfig/openshift_sdn_controller_logs`
//
// ### Released version
// - 4.7.0
//
// ### Backported versions
// - 4.6.21+
//
// ### Changes
// None
func (g *Gatherer) GatherOpenshiftSDNControllerLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := common.LogContainersFilter{
		Namespace:     "openshift-sdn",
		LabelSelector: "app=sdn-controller",
	}
	messagesFilter := common.LogMessagesFilter{
		MessagesToSearch: []string{
			"Node.+is not Ready",
			"Node.+may be offline\\.\\.\\. retrying",
			"Node.+is offline",
			"Node.+is back online",
		},
		IsRegexSearch: true,
		SinceSeconds:  logDefaultSinceSeconds,
		LimitBytes:    logDefaultLimitBytes,
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
