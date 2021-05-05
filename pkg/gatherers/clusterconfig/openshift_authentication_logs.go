package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenshiftAuthenticationLogs collects logs from pods in openshift-authentication namespace with following substring:
//   - "AuthenticationError: invalid resource name"
//
// The Kubernetes API:
//         https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see:
//         https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// * Location in archive: config/pod/openshift-authentication/logs/{pod-name}/errors.log
// * Since versions:
//   * 4.7+
func (g *Gatherer) GatherOpenshiftAuthenticationLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := logContainersFilter{
		namespace:     "openshift-authentication",
		labelSelector: "app=oauth-openshift",
	}
	messagesFilter := logMessagesFilter{
		messagesToSearch: []string{
			"AuthenticationError: invalid resource name",
		},
		isRegexSearch: false,
		sinceSeconds:  86400,     // last day
		limitBytes:    1024 * 64, // maximum 64 kb of logs
	}

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	records, err := gatherLogsFromContainers(
		ctx,
		coreClient,
		containersFilter,
		messagesFilter,
		"errors",
	)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
