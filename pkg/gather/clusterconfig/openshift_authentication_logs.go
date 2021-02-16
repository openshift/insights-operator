package clusterconfig

import (
	"k8s.io/client-go/kubernetes"
)

// GatherOpenshiftAuthenticationLogs collects logs from pods in openshift-authentication namespace with following substring:
//   - "AuthenticationError: invalid resource name"
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// Location in archive: config/pod/openshift-authentication/logs/{pod-name}/errors.log
func GatherOpenshiftAuthenticationLogs(g *Gatherer, c chan<- gatherResult) {
	messagesToSearch := []string{
		"AuthenticationError: invalid resource name",
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := gatherLogsFromPodsInNamespace(
		g.ctx,
		coreClient,
		"openshift-authentication",
		messagesToSearch,
		86400,   // last day
		1024*64, // maximum 64 kb of logs
		"errors",
		"",
		false,
	)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	c <- gatherResult{records, nil}
}
