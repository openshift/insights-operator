package clusterconfig

import (
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenShiftAPIServerOperatorLogs collects logs from openshift-apiserver-operator with following substrings:
//   - "the server has received too many requests and has asked us"
//   - "because serving request timed out and response had been started"
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// Location in archive: config/pod/{namespace-name}/logs/{pod-name}/errors.log
func GatherOpenShiftAPIServerOperatorLogs(g *Gatherer, ch chan<- gatherResult) {
	defer close(ch)
	messagesToSearch := []string{
		"the server has received too many requests and has asked us",
		"because serving request timed out and response had been started",
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		ch <- gatherResult{nil, []error{err}}
		return
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := gatherLogsFromPodsInNamespace(
		g.ctx,
		coreClient,
		"openshift-apiserver-operator",
		messagesToSearch,
		86400,   // last day
		1024*64, // maximum 64 kb of logs
		"errors",
		"app=openshift-apiserver-operator",
	)
	if err != nil {
		ch <- gatherResult{nil, []error{err}}
		return
	}

	ch <- gatherResult{records, nil}
}
