package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
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

	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{errors: []error{err}}
		return
	}

	kubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}

	coreClient := kubeClient.CoreV1()

	records, errs := gatherSAPLicenseManagementLogs(g.ctx, dynamicClient, coreClient, messagesToSearch)
	c <- gatherResult{records, errs}
}

func gatherSAPLicenseManagementLogs(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	coreClient corev1client.CoreV1Interface,
	messagesToSearch []string,
) ([]record.Record, []error) {
	datahubsResource := schema.GroupVersionResource{
		Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs",
	}

	datahubsList, err := dynamicClient.Resource(datahubsResource).List(ctx, metav1.ListOptions{})
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

	var records []record.Record
	var errs []error

	for _, item := range datahubsList.Items {
		namespace := item.GetNamespace()

		namespaceRecords, err := gatherLogsFromContainers(
			ctx,
			coreClient,
			logsContainersFilter{
				namespace:                namespace,
				containerNameRegexFilter: "^vsystem-iptables$",
			},
			logMessagesFilter{
				messagesToSearch: messagesToSearch,
				regexSearch:      false,
				sinceSeconds:     86400,
				limitBytes:       1024 * 64,
			},
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
