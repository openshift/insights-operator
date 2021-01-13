package clusterconfig

import (
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/record"
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
// Location in archive: config/pod/openshift-sdn/logs/{pod-name}/errors.log
func GatherOpenshiftSDNLogs(g *Gatherer) ([]record.Record, []error) {
	messagesToSearch := []string{
		"Got OnEndpointsUpdate for unknown Endpoints",
		"Got OnEndpointsDelete for unknown Endpoints",
		"Unable to update proxy firewall for policy",
		"Failed to update proxy firewall for policy",
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := gatherLogsFromPodsInNamespace(
		g.ctx,
		coreClient,
		"openshift-sdn",
		messagesToSearch,
		86400,   // last day
		1024*64, // maximum 64 kb of logs
		"errors",
		"app=sdn",
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
