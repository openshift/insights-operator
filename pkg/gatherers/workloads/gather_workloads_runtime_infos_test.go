package workloads

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeWokloads(t *testing.T) {

	tests := []struct {
		name     string
		global   workloadRuntimes
		node     workloadRuntimes
		expected workloadRuntimes
	}{
		{
			name:   "global is empty",
			global: map[containerInfo]workloadRuntimeInfoContainer{},
			node: map[containerInfo]workloadRuntimeInfoContainer{
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-foo",
				}: {
					Os:   "linux",
					Kind: "test-kind",
				},
			},
			expected: map[containerInfo]workloadRuntimeInfoContainer{
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-foo",
				}: {
					Os:   "linux",
					Kind: "test-kind",
				},
			},
		},
		{
			name: "global has some existing data",
			global: map[containerInfo]workloadRuntimeInfoContainer{
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-foo",
				}: {
					Os:   "linux",
					Kind: "test-kind-1",
				},
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-bar",
				}: {
					Os:   "windows",
					Kind: "test-kind-2",
				},
				{
					namespace:   "test-B",
					pod:         "pod-B",
					containerID: "container-quz",
				}: {
					Os:   "linux",
					Kind: "test-kind-1",
				},
			},
			node: map[containerInfo]workloadRuntimeInfoContainer{
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-foo",
				}: {
					Os:   "linux",
					Kind: "test-kind-updated",
				},
				{
					namespace:   "test-C",
					pod:         "pod-C",
					containerID: "container-bar-C",
				}: {
					Os:   "linux",
					Kind: "test-kind-updated",
				},
			},
			expected: map[containerInfo]workloadRuntimeInfoContainer{
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-foo",
				}: {
					Os:   "linux",
					Kind: "test-kind-updated",
				},
				{
					namespace:   "test-A",
					pod:         "pod-A",
					containerID: "container-bar",
				}: {
					Os:   "windows",
					Kind: "test-kind-2",
				},
				{
					namespace:   "test-B",
					pod:         "pod-B",
					containerID: "container-quz",
				}: {
					Os:   "linux",
					Kind: "test-kind-1",
				},
				{
					namespace:   "test-C",
					pod:         "pod-C",
					containerID: "container-bar-C",
				}: {
					Os:   "linux",
					Kind: "test-kind-updated",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeWorkloads(tt.global, tt.node)
			assert.Equal(t, tt.expected, tt.global)
		})
	}

}
