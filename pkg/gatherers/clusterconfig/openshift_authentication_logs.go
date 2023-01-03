package clusterconfig

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenshiftAuthenticationLogs Collects logs from pods in `openshift-authentication` namespace with following
// substring:
// - "AuthenticationError: invalid resource name"
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// - docs/insights-archive-sample/config/pod/openshift-authentication/logs/oauth-openshift-6c98668d5b-ftt5n/errors.log
//
// ### Location in archive
// | Version   | Path															|
// | --------- | -------------------------------------------------------------- |
// | >= 4.7    | config/pod/openshift-authentication/logs/{pod-name}/errors.log |
//
// ### Config ID
// `clusterconfig/openshift_authentication_logs`
//
// ### Released version
// 4.7
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherOpenshiftAuthenticationLogs(ctx context.Context) ([]record.Record, []error) {
	containersFilter := common.LogContainersFilter{
		Namespace:     "openshift-authentication",
		LabelSelector: "app=oauth-openshift",
	}
	messagesFilter := common.LogMessagesFilter{
		MessagesToSearch: []string{
			"AuthenticationError: invalid resource name",
		},
		IsRegexSearch: false,
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
