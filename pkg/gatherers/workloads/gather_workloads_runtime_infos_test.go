package workloads

import (
	"context"
	"fmt"
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
			result := getNodeWorkloadRuntimeInfos(ctx, httpServer.URL, "", http.DefaultClient)
			assert.Equal(t, tt.expectedData, result.WorkloadRuntimes)
			if tt.expectedErr != nil {
				assert.Equal(t, tt.expectedErr.Error(), result.Error.Error())
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
			expectedErr:    nil,
			expectedResult: []podWithNodeName(nil),
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
			expectedErr:    nil,
			expectedResult: []podWithNodeName(nil),
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
			expectedErr:    nil,
			expectedResult: []podWithNodeName(nil),
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
			assert.Equal(t, tt.expectedErr, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
