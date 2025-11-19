package clusterconfig

import (
	"context"
	"fmt"
	"regexp"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// GatherQEMUKubeVirtLauncherLogs Collects logs from KubeVirt virt-launcher pods containing QEMU process information.
// This gatherer specifically searches for log lines containing "/usr/libexec/qemu-kvm" to capture QEMU-related
// activity within virtual machines managed by KubeVirt.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/namespaces/default/pods/virt-launcher-example/virt-launcher.json
//
// ### Location in archive
// - `namespaces/{namespace-name}/pods/{pod-name}/virt-launcher.json`
//
// ### Config ID
// `clusterconfig/qemu_kubevirt_launcher_logs`
//
// ### Released version
// - 4.20.0
//
// ### Backported versions
// - 4.19.12+
// - 4.18.25+
// - 4.17.41+
// - 4.16.49+
//
// ### Changes
// 4.21 - bugfix: virt-launcher pods on 'Pending' status caused a gathering error
func (g *Gatherer) GatherQEMUKubeVirtLauncherLogs(ctx context.Context) ([]record.Record, []error) {
	// Setting a fixed value for the maximum number of VMs pods
	const maxVMs int = 100

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	records, err := common.CollectLogsFromContainers(
		ctx, gatherKubeClient.CoreV1(),
		getQEMUArgsContainerFilter(maxVMs),
		getQEMUArgsMessageFilter(),
		func(namespaceName string, podName string, _ string) string {
			return fmt.Sprintf("namespaces/%s/pods/%s/virt-launcher.json", namespaceName, podName)
		})
	if err != nil {
		if _, expected := err.(*common.ContainersSkippedError); expected {
			// Log the warning about our gathering limitation and continue
			klog.Warningf("Some containers were skipped due to reaching the limit: %v", err)
		} else {
			// For other errors, return immediately
			return nil, []error{err}
		}
	}

	records, err = formatKubeVirtRecords(records)
	if err != nil {
		return nil, []error{err}
	}

	return records, []error{}
}

// formatKubeVirtRecords processes log records to extract JSON content from log strings.
// It removes timestamp prefixes and retains only the JSON portion using regex pattern matching.
func formatKubeVirtRecords(records []record.Record) ([]record.Record, error) {
	for i := range records {
		r := records[i]
		log, err := r.Item.Marshal()
		if err != nil {
			return []record.Record{}, err
		}
		// trims the log timestamp prefix e.g. 2025-06-24T13:13:43.473050925Z
		records[i].Item = marshal.Raw{Str: regexp.MustCompile(`(\{.*\})$`).FindString(string(log))}
	}

	return records, nil
}

// getQEMUArgsMessageFilter creates a LogMessagesFilter for filtering QEMU KubeVirt launcher logs
// The MessagesToSearch value "/usr/libexec/qemu-kvm" references the QEMU KVM executable path
// that appears in the command line arguments when KubeVirt creates and manages virtual machines.
func getQEMUArgsMessageFilter() common.LogMessagesFilter {
	return common.LogMessagesFilter{
		IsRegexSearch:    true,
		MessagesToSearch: []string{"/usr/libexec/qemu-kvm"},
	}
}

// getQEMUArgsContainerFilter creates a LogContainersFilter for selecting KubeVirt virt-launcher pods.
// It targets all namespaces using the label selector "kubevirt.io=virt-launcher" to identify relevant containers.
// Adding the field selector "status.phase=Running" filters out pending pods with no compute container or logs.
func getQEMUArgsContainerFilter(maxContainers int) common.LogContainersFilter {
	return common.LogContainersFilter{
		Namespace:              metav1.NamespaceAll,
		LabelSelector:          "kubevirt.io=virt-launcher",
		FieldSelector:          "status.phase=Running",
		MaxNamespaceContainers: maxContainers,
	}
}
