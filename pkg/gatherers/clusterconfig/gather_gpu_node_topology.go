package clusterconfig

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

type gpuNodeInfo struct {
	NodeName         string `json:"nodeName"`
	NvidiaGPUProduct string `json:"nvidiaGPUProduct,omitempty"`
	AMDGPUProduct    string `json:"amdGPUProduct,omitempty"`
	NvidiaGPUCount   string `json:"nvidiaGPUCount,omitempty"`
	AMDGPUCount      string `json:"amdGPUCount,omitempty"`
}

// GatherGPUNodeTopology collects GPU-related labels and capacity fields from all nodes.
// Per node it extracts: nvidia.com/gpu.product and amd.com/gpu.product labels,
// and nvidia.com/gpu and amd.com/gpu capacity values.
// Nodes with no GPU data are omitted. If no GPU nodes are found, no record is created.
//
// ### API Reference
// - https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/node-v1/
//
// ### Sample data
// - docs/insights-archive-sample/config/gpu_node_topology.json
//
// ### Location in archive
// - `config/gpu_node_topology.json`
//
// ### Config ID
// `clusterconfig/gpu_node_topology`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherGPUNodeTopology(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherGPUNodeTopology(ctx, gatherKubeClient.CoreV1())
}

func gatherGPUNodeTopology(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	nodes, err := coreClient.Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var gpuNodes []gpuNodeInfo
	for i := range nodes.Items {
		node := &nodes.Items[i]
		info := gpuNodeInfo{
			NodeName:         node.Name,
			NvidiaGPUProduct: node.Labels["nvidia.com/gpu.product"],
			AMDGPUProduct:    node.Labels["amd.com/gpu.product"],
		}
		if q, ok := node.Status.Capacity[corev1.ResourceName("nvidia.com/gpu")]; ok {
			info.NvidiaGPUCount = q.String()
		}
		if q, ok := node.Status.Capacity[corev1.ResourceName("amd.com/gpu")]; ok {
			info.AMDGPUCount = q.String()
		}
		if info.NvidiaGPUProduct == "" && info.AMDGPUProduct == "" &&
			info.NvidiaGPUCount == "" && info.AMDGPUCount == "" {
			continue
		}
		gpuNodes = append(gpuNodes, info)
	}

	if len(gpuNodes) == 0 {
		return nil, nil
	}

	return []record.Record{
		{
			Name: "config/gpu_node_topology",
			Item: record.JSONMarshaller{Object: gpuNodes},
		},
	}, nil
}
