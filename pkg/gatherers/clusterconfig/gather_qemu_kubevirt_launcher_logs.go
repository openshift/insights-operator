package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (g *Gatherer) GatherQEMUKubeVirtLauncherLogs(ctx context.Context) ([]record.Record, []error) {

	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	// gets the exact line used in the command-line
	filter := common.LogMessagesFilter{IsRegexSearch: true, MessagesToSearch: []string{"/usr/libexec/qemu-kvm"}}

	records, err := common.CollectLogsFromContainers(
		ctx, gatherKubeClient.CoreV1(), common.LogContainersFilter{Namespace: metav1.NamespaceAll, LabelSelector: "kubevirt.io=virt-launcher"}, filter,
		func(_ string, podName string, _ string) string {
			return fmt.Sprintf("aggregated/virt-launcher/logs/%s.log", podName)
		},
	)

	if err != nil {
		return nil, []error{err}
	}

	return records, nil
}
