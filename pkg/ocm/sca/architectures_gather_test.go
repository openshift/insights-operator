package sca

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var testNodes = []v1.Node{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-x86_64",
			Labels: map[string]string{
				// Node marked as control plane
				"node-role.kubernetes.io/control-plane": "",
			},
		},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{
				Architecture: "amd64",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-ppc64le",
		},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{
				Architecture: "ppc64le",
			},
		},
	},
}

func Test_SCAController_GatherMultipleArchitectures(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()

	// Create test nodes
	for _, node := range testNodes {
		_, err := coreClient.Nodes().Create(context.Background(), &node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	expectedArchitectures := map[string]struct{}{
		"x86_64":  {},
		"ppc64le": {},
	}

	scaController := New(coreClient, nil, nil)
	clusterArchitectures, err := scaController.gatherArchitectures(context.Background())
	assert.NoError(t, err, "failed to gather architectures")

	// check the correct control plane arch was found
	assert.Equal(t, "x86_64", clusterArchitectures.ControlPlaneArch, "incorrect control plane architecture")

	assert.Len(t, clusterArchitectures.NodeArchitectures, len(testNodes), "unexpected number of architectures")
	assert.Equal(t, expectedArchitectures, clusterArchitectures.NodeArchitectures, "unexpected architectures")
}

func Test_getArch(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		expected string
	}{
		{
			name:     "amd64 to x86_64",
			arch:     "amd64",
			expected: "x86_64",
		},
		{
			name:     "arm64 to aarch64",
			arch:     "arm64",
			expected: "aarch64",
		},
		{
			name:     "386 to x86",
			arch:     "386",
			expected: "x86",
		},
		{
			name:     "ppc64le stays ppc64le",
			arch:     "ppc64le",
			expected: "ppc64le",
		},
		{
			name:     "s390x stays s390x",
			arch:     "s390x",
			expected: "s390x",
		},
		{
			name:     "ppc stays ppc",
			arch:     "ppc",
			expected: "ppc",
		},
		{
			name:     "ppc64 stays ppc64",
			arch:     "ppc64",
			expected: "ppc64",
		},
		{
			name:     "s390 stays s390",
			arch:     "s390",
			expected: "s390",
		},
		{
			name:     "ia64 stays ia64",
			arch:     "ia64",
			expected: "ia64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getArch(tt.arch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_SCAController_GatherArchitectures_NoControlPlane(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()

	// Create nodes without control plane label
	workerNode := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-node",
		},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{
				Architecture: "amd64",
			},
		},
	}

	_, err := coreClient.Nodes().Create(context.Background(), &workerNode, metav1.CreateOptions{})
	assert.NoError(t, err)

	scaController := New(coreClient, nil, nil)
	clusterArchitectures, err := scaController.gatherArchitectures(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, "", clusterArchitectures.ControlPlaneArch)
	assert.Len(t, clusterArchitectures.NodeArchitectures, 1)
	assert.Contains(t, clusterArchitectures.NodeArchitectures, "x86_64")
}

func Test_SCAController_GatherArchitectures_EmptyCluster(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()
	scaController := New(coreClient, nil, nil)

	clusterArchitectures, err := scaController.gatherArchitectures(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, "", clusterArchitectures.ControlPlaneArch)
	assert.Len(t, clusterArchitectures.NodeArchitectures, 0)
}
