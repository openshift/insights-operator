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
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GatherQEMUKubeVirtLauncherLogs Collects logs from KubeVirt virt-launcher pods containing QEMU process information.
// This gatherer specifically searches for log lines containing "/usr/libexec/qemu-kvm" to capture QEMU-related
// activity within virtual machines managed by KubeVirt.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/aggregated/virt-launcher/logs/virt-launcher-centos-stream9-5hvrs.json
//
// ### Location in archive
// - `aggregated/virt-launcher/logs/{pod-name}.json`
//
// ### Config ID
// `clusterconfig/qemu_kubevirt_launcher_logs`
//
// ### Released version
// - 4.20.0
//
// ### Backported versions (TBD)
// - 4.19.z
// - 4.18.z
// - 4.17.z
// - 4.16.z
//
// ### Changes
// None
func (g *Gatherer) GatherQEMUKubeVirtLauncherLogs(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	records, err := gatherQEMUKubeVirtLauncherLogs(ctx, gatherKubeClient.CoreV1())
	if err != nil {
		return nil, []error{err}
	}

	records, err = formatKubeVirtRecords(records)
	if err != nil {
		return nil, []error{err}
	}

	return records, nil
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

// gatherQEMUKubeVirtLauncherLogs collects QEMU KubeVirt launcher logs from the given CoreV1Interface
// This function is extracted for testability and accepts a client interface for mocking
func gatherQEMUKubeVirtLauncherLogs(ctx context.Context, coreClient v1.CoreV1Interface) ([]record.Record, error) {
	return common.CollectLogsFromContainers(
		ctx, coreClient, getQEMUArgsContainerFilter(), getQEMUArgsMessageFilter(),
		func(_ string, podName string, _ string) string {
			return fmt.Sprintf("aggregated/virt-launcher/logs/%s.json", podName)
		},
	)
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
func getQEMUArgsContainerFilter() common.LogContainersFilter {
	return common.LogContainersFilter{
		Namespace:     metav1.NamespaceAll,
		LabelSelector: "kubevirt.io=virt-launcher",
	}
}
