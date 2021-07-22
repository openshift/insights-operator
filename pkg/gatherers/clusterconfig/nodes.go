package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherNodes collects all Nodes.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/node.go#L78
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#nodelist-v1core
//
// * Location in archive: config/node/
// * Id in config: nodes
func (g *Gatherer) GatherNodes(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherNodes(ctx, gatherKubeClient.CoreV1())
}

func gatherNodes(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	nodes, err := coreClient.Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	records := make([]record.Record, 0, len(nodes.Items))
	for i := range nodes.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/node/%s", nodes.Items[i].Name),
			Item: record.ResourceMarshaller{Resource: anonymizeNode(&nodes.Items[i])}})
	}
	return records, nil
}

func anonymizeNode(node *corev1.Node) *corev1.Node {
	for k := range node.Annotations {
		if isProductNamespacedKey(k) {
			continue
		}
		node.Annotations[k] = ""
	}
	for k, v := range node.Labels {
		if isProductNamespacedKey(k) {
			continue
		}
		node.Labels[k] = anonymize.String(v)
	}
	node.Status.NodeInfo.BootID = anonymize.String(node.Status.NodeInfo.BootID)
	node.Status.NodeInfo.SystemUUID = anonymize.String(node.Status.NodeInfo.SystemUUID)
	node.Status.NodeInfo.MachineID = anonymize.String(node.Status.NodeInfo.MachineID)
	node.Status.Images = nil
	return node
}

func isProductNamespacedKey(key string) bool {
	return strings.Contains(key, "openshift.io/") || strings.Contains(key, "k8s.io/") || strings.Contains(key, "kubernetes.io/")
}
