package workloads

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestGetNodeWorkloadRuntimeInfos(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		status       int
		expectedErr  error
		expectedData workloadRuntimes
	}{
		{
			name:         "data cannot be parsed",
			data:         []byte("this is not json"),
			status:       http.StatusOK,
			expectedErr:  fmt.Errorf("invalid character 'h' in literal true (expecting 'r')"),
			expectedData: nil,
		},
		{
			name:         "server returns non-200 HTTP response",
			data:         []byte("this is not json"),
			status:       http.StatusInternalServerError,
			expectedErr:  fmt.Errorf("%d %s", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)),
			expectedData: nil,
		},
		{
			name:        "server returns 200 HTTP response with data",
			data:        []byte(`{"test-namespace": {"test-pod-1": {"cri-o://foo-1": {"os": "rhel", "runtimes": [{"name": "runtime-A"}]}}}}`),
			status:      http.StatusOK,
			expectedErr: nil,
			expectedData: workloadRuntimes{
				containerInfo{
					namespace:   "test-namespace",
					pod:         "test-pod-1",
					containerID: "cri-o://foo-1",
				}: workloadRuntimeInfoContainer{
					Os: "rhel",
					Runtimes: []RuntimeComponent{
						{
							Name: "runtime-A",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, err := w.Write(tt.data)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			ctx := context.Background()
			result := getNodeWorkloadRuntimeInfos(ctx, httpServer.URL)
			assert.Equal(t, tt.expectedData, result.WorkloadRuntimes)
			if tt.expectedErr != nil {
				assert.Equal(t, tt.expectedErr.Error(), result.Error.Error())
			}
		})
	}
}
