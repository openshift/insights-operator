package clusterconfig

import (
	"k8s.io/client-go/kubernetes"
)

// GatherOpenshiftSDNLogs collects logs from pods in openshift-sdn namespace with following substrings:
//   - "Got OnEndpointsUpdate for unknown Endpoints",
//   - "Got OnEndpointsDelete for unknown Endpoints",
//   - "Unable to update proxy firewall for policy",
//   - "Failed to update proxy firewall for policy",
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/openshift-sdn/logs/{pod-name}/errors.log
// * Since versions:
//   * 4.6.19+
//   * 4.7+
func GatherOpenshiftSDNLogs(g *Gatherer, c chan<- gatherResult) {
	defer close(c)

	containersFilter := logContainersFilter{
		namespace:     "openshift-sdn",
		labelSelector: "app=sdn",
	}
	messagesFilter := logMessagesFilter{
		messagesToSearch: []string{
			"Got OnEndpointsUpdate for unknown Endpoints",
			"Got OnEndpointsDelete for unknown Endpoints",
			"Unable to update proxy firewall for policy",
			"Failed to update proxy firewall for policy",
		},
		isRegexSearch: false,
		sinceSeconds:  86400,
		limitBytes:    1024 * 64,
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
		containersFilter,
		messagesFilter,
		"errors",
	)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	c <- gatherResult{records, nil}
}
