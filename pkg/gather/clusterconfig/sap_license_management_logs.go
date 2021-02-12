package clusterconfig

import (
	"k8s.io/client-go/kubernetes"
)

// GatherSAPLicenseManagementLogs collects logs from license management pods with the following substrings:
//   - "can't initialize iptables table",
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// Location in archive: config/pod/sdi/logs/{pod-name}/errors.log
func GatherSAPLicenseManagementLogs(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	messagesToSearch := []string{
		"can't initialize iptables table",
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
		"sdi",
		messagesToSearch,
		false,
		86400,   // last day
		1024*64, // maximum 64 kb of logs
		"errors",
		"",
		"^license-manager.*$",
	)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	c <- gatherResult{records, nil}
}
