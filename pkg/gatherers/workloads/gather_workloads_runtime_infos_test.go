package workloads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
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
		name               string
		data               []byte
		status             int
		token              string
		expectedErr        error
		expectedData       workloadRuntimes
		checkAuthHeader    bool
		expectedAuthHeader string
		verifyDetails      func(t *testing.T, result workloadRuntimes)
	}{
		{
			name:         "invalid JSON data",
			data:         []byte("this is not json"),
			status:       http.StatusOK,
			expectedErr:  fmt.Errorf("invalid character 'h' in literal true (expecting 'r')"),
			expectedData: nil,
		},
		{
			name:         "non-200 HTTP status",
			data:         []byte("server error"),
			status:       http.StatusInternalServerError,
			expectedErr:  fmt.Errorf("%d %s", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)),
			expectedData: nil,
		},
		{
			name:        "valid data with single container",
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
						{Name: "runtime-A"},
					},
				},
			},
		},
		{
			name: "empty containers are skipped",
			data: []byte(`{"test-namespace": {"test-pod-1": {"cri-o://foo-1": ` +
				`{"os": "rhel", "runtimes": [{"name": "runtime-A"}]}, "cri-o://empty": {}}}}`),
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
						{Name: "runtime-A"},
					},
				},
			},
		},
		{
			name:               "authorization header is set",
			data:               []byte(`{"test-namespace": {"test-pod-1": {"cri-o://foo-1": {"os": "rhel"}}}}`),
			status:             http.StatusOK,
			token:              "test-token-12345",
			checkAuthHeader:    true,
			expectedAuthHeader: "Bearer test-token-12345",
			expectedData:       nil,
		},
		{
			name:   "complex nested data with multiple namespaces",
			status: http.StatusOK,
			data: []byte(`{
				"namespace-1": {
					"pod-1": {
						"cri-o://container-1": {
							"os": "rhel",
							"kind": "vm",
							"runtimes": [
								{"name": "runtime-A", "version": "1.0"},
								{"name": "runtime-B", "version": "2.0"}
							]
						},
						"cri-o://container-2": {
							"os": "ubuntu",
							"runtimes": []
						}
					},
					"pod-2": {
						"cri-o://container-3": {}
					}
				},
				"namespace-2": {
					"pod-3": {
						"cri-o://container-4": {
							"os": "fedora"
						}
					}
				}
			}`),
			verifyDetails: func(t *testing.T, result workloadRuntimes) {
				assert.Equal(t, 3, len(result), "should have 3 containers (container-3 is empty and skipped)")

				container1 := containerInfo{namespace: "namespace-1", pod: "pod-1", containerID: "cri-o://container-1"}
				assert.Contains(t, result, container1)
				assert.Equal(t, "rhel", result[container1].Os)
				assert.Equal(t, "vm", result[container1].Kind)
				assert.Equal(t, 2, len(result[container1].Runtimes))

				container2 := containerInfo{namespace: "namespace-1", pod: "pod-1", containerID: "cri-o://container-2"}
				assert.Contains(t, result, container2)
				assert.Equal(t, "ubuntu", result[container2].Os)
				assert.Equal(t, 0, len(result[container2].Runtimes))

				container3 := containerInfo{namespace: "namespace-1", pod: "pod-2", containerID: "cri-o://container-3"}
				assert.NotContains(t, result, container3, "empty container should be skipped")

				container4 := containerInfo{namespace: "namespace-2", pod: "pod-3", containerID: "cri-o://container-4"}
				assert.Contains(t, result, container4)
				assert.Equal(t, "fedora", result[container4].Os)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedAuthHeader string
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify it's a POST request with correct content type
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				if tt.checkAuthHeader {
					receivedAuthHeader = r.Header.Get("Authorization")
				}
				w.WriteHeader(tt.status)
				_, err := w.Write(tt.data)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer httpServer.Close()

			ctx := context.Background()
			// Pass container IDs to the function
			containerIDs := []string{"cri-o://test-container-1", "cri-o://test-container-2"}
			result := getNodeWorkloadRuntimeInfos(ctx, httpServer.URL, tt.token, http.DefaultClient, containerIDs)

			if tt.expectedErr != nil {
				assert.Contains(t, result.Error.Error(), tt.expectedErr.Error())
				assert.Nil(t, result.WorkloadRuntimes)
			} else {
				assert.Nil(t, result.Error)
				if tt.verifyDetails != nil {
					tt.verifyDetails(t, result.WorkloadRuntimes)
				} else if tt.expectedData != nil {
					assert.Equal(t, tt.expectedData, result.WorkloadRuntimes)
				}
			}

			if tt.checkAuthHeader {
				assert.Equal(t, tt.expectedAuthHeader, receivedAuthHeader)
			}
		})
	}
}

func TestGetInsightsOperatorRuntimePodIPs(t *testing.T) {
	tests := []struct {
		name           string
		pods           []*v1.Pod
		expectedErr    error
		expectedResult []podWithNodeName
	}{
		{
			name:           "empty Pod list",
			pods:           []*v1.Pod{},
			expectedErr:    fmt.Errorf("no running pods found for the insights-runtime-extractor statefulset"),
			expectedResult: nil,
		},
		{
			name: "Pod doesn't have the required label",
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "openshift-insights",
					},
				},
			},
			expectedErr:    fmt.Errorf("no running pods found for the insights-runtime-extractor statefulset"),
			expectedResult: nil,
		},
		{
			name: "Pod has the required label, but it is not running",
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "openshift-insights",
						Labels: map[string]string{
							"app.kubernetes.io/name": "insights-runtime-extractor",
						},
					},
				},
			},
			expectedErr:    fmt.Errorf("no running pods found for the insights-runtime-extractor statefulset"),
			expectedResult: nil,
		},
		{
			name: "some Pods found",
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "openshift-insights",
						Labels: map[string]string{
							"app.kubernetes.io/name": "insights-runtime-extractor",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-2",
						Namespace: "openshift-insights",
						Labels: map[string]string{
							"app.kubernetes.io/name": "insights-runtime-extractor",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node-foo",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						PodIP: "127.0.0.1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-3",
						Namespace: "openshift-another",
						Labels: map[string]string{
							"app.kubernetes.io/name": "insights-runtime-extractor",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node-bar",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						PodIP: "127.0.0.1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-4",
						Namespace: "openshift-insights",
						Labels: map[string]string{
							"app.kubernetes.io/name": "insights-runtime-extractor",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node-bar",
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						PodIP: "127.0.0.10",
					},
				},
			},
			expectedErr: nil,
			expectedResult: []podWithNodeName{
				{
					nodeName: "node-foo",
					podIP:    "127.0.0.1",
				},
				{
					nodeName: "node-bar",
					podIP:    "127.0.0.10",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := kubefake.NewSimpleClientset()
			err := utils.AddObjectsToClientSet[[]*v1.Pod](cli, tt.pods)
			assert.NoError(t, err)
			err = os.Setenv("POD_NAMESPACE", "openshift-insights")
			assert.NoError(t, err)
			result, err := getInsightsOperatorRuntimePodIPs(context.Background(), cli.CoreV1())
			if tt.expectedErr == nil {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGatherWorkloadRuntimeInfos_NoPods(t *testing.T) {
	cli := kubefake.NewSimpleClientset()
	err := os.Setenv("POD_NAMESPACE", "openshift-insights")
	assert.NoError(t, err)

	ctx := context.Background()
	// Pass empty containerIDsByNode
	containersByNode := make(containerIDsByNode)
	result, errors := gatherWorkloadRuntimeInfos(ctx, cli.CoreV1(), containersByNode)

	assert.Nil(t, result)
	assert.Len(t, errors, 1)
	assert.Contains(t, errors[0].Error(), "no running pods found")
}

func TestGetNodeWorkloadRuntimeInfos_POSTRequestBody(t *testing.T) {
	var receivedBody []byte
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST request
		assert.Equal(t, http.MethodPost, r.Method)
		// Read and save the request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		receivedBody = body
		// Return empty response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer httpServer.Close()

	ctx := context.Background()
	containerIDs := []string{"cri-o://container-1", "cri-o://container-2", "cri-o://container-3"}
	_ = getNodeWorkloadRuntimeInfos(ctx, httpServer.URL, "test-token", http.DefaultClient, containerIDs)

	// Verify the request body contains the container IDs
	var reqBody gatherRuntimeInfoRequest
	err := json.Unmarshal(receivedBody, &reqBody)
	assert.NoError(t, err)
	assert.Equal(t, containerIDs, reqBody.ContainerIDs)
}
