package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSAPVsystemIptablesLogs Collects logs from SAP `vsystem-iptables` containers
// including one from license management pods with the following substring:
//   - "can't initialize iptables table",
//
// > **Note**
// > This data is collected only if the `installers.datahub.sap.com` resource is found in the cluster.
//
// ### API Reference
// - https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/pod_expansion.go#L48
// - https://docs.openshift.com/container-platform/4.6/rest_api/workloads_apis/pod-core-v1.html#apiv1namespacesnamespacepodsnamelog
//
// ### Sample data
// - docs/insights-archive-sample/config/pod/sdi/logs/license-manager-da1d2e8fadfb8dd7022f08-4hjh7-6887768c5b-qzxb6/errors.log
//
// ### Location in archive
// - `config/pod/{namespace}/logs/{pod-name}/errors.log`
//
// ### Config ID
// `clusterconfig/sap_license_management_logs`
//
// ### Released version
// - 4.8.2
//
// ### Backported versions
// - 4.7.5+
// - 4.6.25+
//
// ### Changes
// None
func (g *Gatherer) GatherSAPVsystemIptablesLogs(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	datahubsList, err := dynamicClient.Resource(datahubGroupVersionResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		klog.Info("SAP resources weren't found")
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	// If no DataHubs resource exists on the cluster, skip this gathering.
	// This may already be handled by the IsNotFound check, but it's better to be sure.
	if len(datahubsList.Items) == 0 {
		klog.Info("SAP resources weren't found")
		return nil, nil
	}

	coreClient := kubeClient.CoreV1()

	return gatherSAPLicenseManagementLogs(ctx, coreClient, datahubsList.Items)
}

func gatherSAPLicenseManagementLogs(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	datahubs []unstructured.Unstructured,
) ([]record.Record, []error) {
	var records []record.Record
	var errs []error

	for _, item := range datahubs {
		containersFilter := common.LogContainersFilter{
			Namespace:                item.GetNamespace(),
			ContainerNameRegexFilter: "^vsystem-iptables$",
		}
		messagesFilter := common.LogMessagesFilter{
			MessagesToSearch: []string{
				"can't initialize iptables table",
			},
			IsRegexSearch: false,
			SinceSeconds:  logDefaultSinceSeconds,
			LimitBytes:    logDefaultLimitBytes,
		}

		namespaceRecords, err := common.CollectLogsFromContainers(
			ctx,
			coreClient,
			containersFilter,
			messagesFilter,
			nil,
		)
		if err != nil {
			errs = append(errs, err)
		} else {
			records = append(records, namespaceRecords...)
		}
	}

	return records, errs
}
