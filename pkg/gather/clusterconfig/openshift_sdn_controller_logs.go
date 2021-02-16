package clusterconfig

import (
	"k8s.io/client-go/kubernetes"
)

// GatherOpenshiftSDNControllerLogs collects logs from sdn-controller pod in openshift-sdn namespace with following substrings:
//   - "Node %s is not Ready": A node has been set offline for egress IPs because it is reported not ready at API
//   - "Node %s may be offline... retrying": An egress node has failed the egress IP health check once,
//       so it has big chances to be marked as offline soon or, at the very least, there has been a connectivity glitch.
//   - "Node %s is offline": An egress node has failed enough probes to have been marked offline for egress IPs.
//       If it has egress CIDRs assigned, its egress IPs have been moved to other nodes.
//       Indicates issues at either the node or the network between the master and the node.
//   - "Node %s is back online": This indicates that a node has recovered from the condition described
//       at the previous message, by starting succeeding the egress IP health checks.
//       Useful just in case that previous “Node %s is offline” messages are lost,
//       so that we have a clue that there was failure previously.
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// Location in archive: config/pod/openshift-sdn/logs/{pod-name}/errors.log
func GatherOpenshiftSDNControllerLogs(g *Gatherer, c chan<- gatherResult) {
	messagesToSearch := []string{
		"Node.+is not Ready",
		"Node.+may be offline\\.\\.\\. retrying",
		"Node.+is offline",
		"Node.+is back online",
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := gatherLogsFromContainers(
		g.ctx,
		coreClient,
		logsContainersFilter{
			namespace:     "openshift-sdn",
			labelSelector: "app=sdn-controller",
		},
		logMessagesFilter{
			messagesToSearch: messagesToSearch,
			regexSearch:      true,
			sinceSeconds:     86400,     // last day
			limitBytes:       1024 * 64, // maximum 64 kb of logs
		},
		"errors",
	)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	c <- gatherResult{records, nil}
	return
}
