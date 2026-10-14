package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/record"
)

func Test_gatherGPUNodeTopology(t *testing.T) {
	tests := []struct {
		name          string
		nodes         *corev1.NodeList
		wantRecords   []record.Record
		wantErrsCount int
	}{
		{
			name: "nodes with nvidia and amd gpu, non-gpu node excluded",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "gpu-node-nvidia",
							Labels: map[string]string{"nvidia.com/gpu.product": "Tesla-T4"},
						},
						Status: corev1.NodeStatus{
							Capacity: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "gpu-node-amd",
							Labels: map[string]string{"amd.com/gpu.product": "Instinct-MI250"},
						},
						Status: corev1.NodeStatus{
							Capacity: corev1.ResourceList{
								corev1.ResourceName("amd.com/gpu"): resource.MustParse("2"),
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cpu-only-node",
						},
					},
				},
			},
			wantRecords: []record.Record{
				{
					Name: "config/gpu_node_topology",
					Item: record.JSONMarshaller{Object: []gpuNodeInfo{
						{NodeName: "gpu-node-amd", AMDGPUProduct: "Instinct-MI250", AMDGPUCount: "2"},
						{NodeName: "gpu-node-nvidia", NvidiaGPUProduct: "Tesla-T4", NvidiaGPUCount: "4"},
					}},
				},
			},
			wantErrsCount: 0,
		},
		{
			name:          "no nodes",
			nodes:         &corev1.NodeList{},
			wantRecords:   nil,
			wantErrsCount: 0,
		},
		{
			name: "nodes exist but none have gpu data",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "worker-2"}},
				},
			},
			wantRecords:   nil,
			wantErrsCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreClient := fake.NewClientset(tt.nodes)
			records, errs := gatherGPUNodeTopology(context.Background(), coreClient.CoreV1())
			assert.Equal(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrsCount)
		})
	}
}
