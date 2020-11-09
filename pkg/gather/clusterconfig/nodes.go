package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherNodes collects all Nodes.
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/node.go#L78
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#nodelist-v1core
//
// Location in archive: config/node/
func GatherNodes(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		nodes, err := g.coreClient.Nodes().List(g.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, []error{err}
		}
		records := make([]record.Record, 0, len(nodes.Items))
		for i, node := range nodes.Items {
			records = append(records, record.Record{Name: fmt.Sprintf("config/node/%s", node.Name), Item: NodeAnonymizer{&nodes.Items[i]}})
		}
		return records, nil
	}
}

// NodeAnonymizer implements serialization of Node with anonymization
type NodeAnonymizer struct{ *corev1.Node }

// Marshal implements serialization of Node with anonymization
func (a NodeAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, anonymizeNode(a.Node))
}

// GetExtension returns extension for anonymized node objects
func (a NodeAnonymizer) GetExtension() string {
	return "json"
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
		node.Labels[k] = anonymizeString(v)
	}
	for i := range node.Status.Addresses {
		node.Status.Addresses[i].Address = anonymizeURL(node.Status.Addresses[i].Address)
	}
	node.Status.NodeInfo.BootID = anonymizeString(node.Status.NodeInfo.BootID)
	node.Status.NodeInfo.SystemUUID = anonymizeString(node.Status.NodeInfo.SystemUUID)
	node.Status.NodeInfo.MachineID = anonymizeString(node.Status.NodeInfo.MachineID)
	node.Status.Images = nil
	return node
}

func isProductNamespacedKey(key string) bool {
	return strings.Contains(key, "openshift.io/") || strings.Contains(key, "k8s.io/") || strings.Contains(key, "kubernetes.io/")
}
