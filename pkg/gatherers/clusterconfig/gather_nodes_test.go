package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/utils/anonymize"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/kubernetes/fake"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

func Test_gatherNodes(t *testing.T) {
	tests := []struct {
		name          string
		nodes         *corev1.NodeList
		wantRecords   []record.Record
		wantErrsCount int
	}{
		{
			name: "successful retrieval nodes",
			nodes: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
						},
					},
				},
			},
			wantRecords: []record.Record{
				{
					Name: "config/node/node1",
					Item: record.ResourceMarshaller{Resource: &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
						},
					}},
				},
				{
					Name: "config/node/node2",
					Item: record.ResourceMarshaller{Resource: &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
						},
					}},
				},
			},
			wantErrsCount: 0,
		},
		{
			name:          "nodes not found",
			nodes:         &corev1.NodeList{},
			wantRecords:   []record.Record{},
			wantErrsCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreClient := fake.NewSimpleClientset(tt.nodes)
			records, errs := gatherNodes(context.TODO(), coreClient.CoreV1())
			assert.Equal(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrsCount)
		})
	}
}

func Test_isProductNamespacedKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "contains openshift.io/",
			key:  "openshift.io/something",
			want: true,
		},
		{
			name: "contains k8s.io/",
			key:  "k8s.io/something",
			want: true,
		},
		{
			name: "contains kubernetes.io/",
			key:  "kubernetes.io/something",
			want: true,
		},
		{
			name: "does not contain anything",
			key:  "something",
			want: false,
		},
	}

	for _, tt := range tests {
		result := isProductNamespacedKey(tt.key)
		assert.Equal(t, tt.want, result)
	}
}

func Test_isRegionLabel(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "key with 'failure-domain.beta.kubernetes.io/region'",
			key:  "failure-domain.beta.kubernetes.io/region",
			want: true,
		},
		{
			name: "key with 'topology.kubernetes.io/region'",
			key:  "topology.kubernetes.io/region",
			want: true,
		},
		{
			name: "key without 'failure-domain.beta.kubernetes.io/region' or 'topology.kubernetes.io/region'",
			key:  "other/key",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRegionLabel(tt.key)
			assert.Equal(t, tt.want, result)
		})
	}
}

func Test_anonymizeNode(t *testing.T) {
	tests := []struct {
		name string
		node *corev1.Node
		want *corev1.Node
	}{
		{
			name: "successful anonymize node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: map[string]string{"annot-key": "annot-value"},
					Labels:      map[string]string{"label-key": "label-value"},
				},
				Status: corev1.NodeStatus{
					NodeInfo: corev1.NodeSystemInfo{
						BootID:          "boot-id",
						SystemUUID:      "system-uuid",
						MachineID:       "machine-id",
						OperatingSystem: "operating-system",
						Architecture:    "architecture",
					},
					Images: []corev1.ContainerImage{
						{
							Names:     []string{"image-name"},
							SizeBytes: 123,
						},
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: map[string]string{"annot-key": ""},
					Labels:      map[string]string{"label-key": anonymize.String("label-value")},
				},
				Status: corev1.NodeStatus{
					NodeInfo: corev1.NodeSystemInfo{
						BootID:          anonymize.String("boot-id"),
						SystemUUID:      anonymize.String("system-uuid"),
						MachineID:       anonymize.String("machine-id"),
						OperatingSystem: "operating-system",
						Architecture:    "architecture",
					},
					Images: nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, anonymizeNode(tt.node))
		})
	}
}
