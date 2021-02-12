package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPVsystemIptablesLogs collects logs from SAP vsystem-iptables containers
// including one from license management pods with the following substring:
//   - "can't initialize iptables table",
//
// The Kubernetes API https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// Response see https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// Location in archive: config/pod/{namespace}/logs/{pod-name}/errors.log
func GatherSAPVsystemIptablesLogs(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
		if err != nil {
			return nil, []error{err}
		}

		kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
		if err != nil {
			return nil, []error{err}
		}

		datahubsList, err := dynamicClient.Resource(datahubGroupVersionResource).List(g.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		// If no DataHubs resource exists on the cluster, skip this gathering.
		// This may already be handled by the IsNotFound check, but it's better to be sure.
		if len(datahubsList.Items) == 0 {
			return nil, nil
		}

		coreClient := kubeClient.CoreV1()

		records, errs := gatherSAPLicenseManagementLogs(g.ctx, coreClient, datahubsList.Items)
		return records, errs
	}
}

func gatherSAPLicenseManagementLogs(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	datahubs []unstructured.Unstructured,
) ([]record.Record, []error) {
	var records []record.Record
	var errs []error

	for _, item := range datahubs {
		containersFilter := logContainersFilter{
			namespace:                item.GetNamespace(),
			containerNameRegexFilter: "^vsystem-iptables$",
		}
		messagesFilter := logMessagesFilter{
			messagesToSearch: []string{
				"can't initialize iptables table",
			},
			isRegexSearch: false,
			sinceSeconds:  86400,
			limitBytes:    1024 * 64,
		}

		namespaceRecords, err := gatherLogsFromContainers(
			ctx,
			coreClient,
			containersFilter,
			messagesFilter,
			"errors",
		)
		if err != nil {
			errs = append(errs, err)
		} else {
			records = append(records, namespaceRecords...)
		}
	}

	return records, errs
}
