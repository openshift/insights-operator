package workloads

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

// nolint: funlen, gocyclo, gosec
func Test_gatherWorkloadInfo(t *testing.T) {
	if len(os.Getenv("TEST_INTEGRATION")) == 0 {
		t.Skip("will not run unless TEST_INTEGRATION is set, and requires KUBECONFIG to point to a real cluster")
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	config.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	config.ContentType = "application/vnd.kubernetes.protobuf"

	g := New(nil, config)
	ctx := context.TODO()
	start := time.Now()
	records, errs := g.GatherWorkloadInfo(ctx)
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	t.Logf("Gathered in %s", time.Since(start).Round(time.Second).String())

	if len(records) != 1 {
		t.Fatalf("unexpected: %v", records)
	}
	for _, r := range records {
		out, err := json.MarshalIndent(r.Item.(record.JSONMarshaller).Object, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile("../../../docs/insights-archive-sample/config/workload_info.json", out, 0750); err != nil {
			t.Fatal(err)
		}

		out, err = json.Marshal(r.Item)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(out); err != nil {
			t.Fatal(err)
		}
		if err := gw.Close(); err != nil {
			t.Fatal(err)
		}

		images := make(map[string]struct{})

		var total, totalTerminal, totalIgnored, totalInvalid int
		pods := r.Item.(record.JSONMarshaller).Object.(*workloadPods)
		for ns, pods := range pods.Namespaces {
			var count int
			for i, pod := range pods.Shapes {
				count += pod.Duplicates + 1
				if len(pod.Containers) == 0 {
					t.Errorf("%s.Shapes[%d] should not have a shape with empty containers: %#v", ns, i, pod)
				}
				for j, container := range pod.InitContainers {
					if len(container.ImageID) == 0 {
						t.Errorf("%s.Shapes[%d].InitContainers[%d] should have an imageID: %#v", ns, i, j, pod)
					}
					images[container.ImageID] = struct{}{}
				}
				for j, container := range pod.Containers {
					if len(container.ImageID) == 0 {
						t.Errorf("%s.Shapes[%d].Containers[%d] should have an imageID: %#v", ns, i, j, pod)
					}
					images[container.ImageID] = struct{}{}
				}
			}
			if (count + pods.TerminalCount + pods.InvalidCount + pods.IgnoredCount) != pods.Count {
				t.Errorf("%s had mismatched count of pods", ns)
			}
			total += pods.Count
			totalTerminal += pods.TerminalCount
			totalIgnored += pods.IgnoredCount
			totalInvalid += pods.InvalidCount
		}
		if pods.PodCount != total {
			t.Errorf("mismatched pod count %d vs %d", pods.PodCount, total)
		}

		var totalImagesWithData int
		for imageID, image := range pods.Images {
			totalImagesWithData++
			if len(image.LayerIDs) == 0 {
				t.Errorf("found empty layer IDs in image %s", imageID)
			}
		}
		if pods.ImageCount != len(images) {
			t.Errorf("total image count did not match counted images %d vs %d", pods.ImageCount, len(images))
		}
		if totalImagesWithData > pods.ImageCount {
			t.Errorf("found more images than exist %d vs %d", totalImagesWithData, pods.ImageCount)
		}

		t.Logf(`
  uncompressed: %10d bytes
    compressed: %10d bytes (%.1f%%)

    namespaces: %5d

          pods: %5d
      terminal: %5d (%.1f%%)
       ignored: %5d (%.1f%%)
       invalid: %5d (%.1f%%)

        images: %5d
        w/data: %5d (%.1f%%)
        cached: %5d
`,
			len(out),
			buf.Len(),
			float64(buf.Len())/float64(len(out))*100,
			len(pods.Namespaces),
			total,
			totalTerminal,
			float64(totalTerminal)/float64(total)*100,
			totalIgnored,
			float64(totalIgnored)/float64(total)*100,
			totalInvalid,
			float64(totalInvalid)/float64(total)*100,
			pods.ImageCount,
			totalImagesWithData,
			float64(totalImagesWithData)/float64(pods.ImageCount)*100,
			workloadImageLRU.Len(),
		)
	}
}

func Test_getExternalImageRepo(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Image repository under the Red Hat domain will be ignored",
			url:      "registry.redhat.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:abc",
			expected: "",
		},
		{
			name:     "Image repository outside the Red Hat domain is returned",
			url:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:abc",
			expected: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:abc",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			test := getExternalImageRepo(testCase.url)

			// Assert
			assert.Equal(t, testCase.expected, test)
		})
	}
}

func Test_podCanBeIgnored(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		ignored bool
	}{
		{
			name: "running pod with all containers",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "init"}},
					Containers:     []corev1.Container{{Name: "app"}},
				},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{{Name: "init"}},
					ContainerStatuses:     []corev1.ContainerStatus{{Name: "app"}},
				},
			},
			ignored: false,
		},
		{
			name: "terminal pod phases are ignored",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
				Status: corev1.PodStatus{
					Phase:             corev1.PodSucceeded,
					ContainerStatuses: []corev1.ContainerStatus{{Name: "app"}},
				},
			},
			ignored: true,
		},
		{
			name: "missing container status is ignored",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app1"}, {Name: "app2"}},
				},
				Status: corev1.PodStatus{
					Phase:             corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{{Name: "app1"}},
				},
			},
			ignored: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := podCanBeIgnored(tt.pod)
			assert.Equal(t, tt.ignored, result)
		})
	}
}

func Test_workloadContainerShapesEqual(t *testing.T) {
	tests := []struct {
		name  string
		a     []workloadContainerShape
		b     []workloadContainerShape
		equal bool
	}{
		{
			name: "identical single container shapes are equal",
			a: []workloadContainerShape{
				{ImageID: "sha256:abc", FirstCommand: "cmd1", FirstArg: "arg1"},
			},
			b: []workloadContainerShape{
				{ImageID: "sha256:abc", FirstCommand: "cmd1", FirstArg: "arg1"},
			},
			equal: true,
		},
		{
			name: "different image IDs are not equal",
			a: []workloadContainerShape{
				{ImageID: "sha256:abc", FirstCommand: "cmd1", FirstArg: "arg1"},
			},
			b: []workloadContainerShape{
				{ImageID: "sha256:def", FirstCommand: "cmd1", FirstArg: "arg1"},
			},
			equal: false,
		},
		{
			name: "different lengths are not equal",
			a: []workloadContainerShape{
				{ImageID: "sha256:abc"},
			},
			b: []workloadContainerShape{
				{ImageID: "sha256:abc"},
				{ImageID: "sha256:def"},
			},
			equal: false,
		},
		{
			name: "multiple identical containers are equal",
			a: []workloadContainerShape{
				{ImageID: "sha256:abc", FirstCommand: "cmd1"},
				{ImageID: "sha256:def", FirstArg: "arg1"},
			},
			b: []workloadContainerShape{
				{ImageID: "sha256:abc", FirstCommand: "cmd1"},
				{ImageID: "sha256:def", FirstArg: "arg1"},
			},
			equal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := workloadContainerShapesEqual(tt.a, tt.b)
			assert.Equal(t, tt.equal, result)
		})
	}
}

func Test_workloadHashString(t *testing.T) {
	h := sha256.New()

	hash1 := workloadHashString(h, "test1")
	hash2 := workloadHashString(h, "test2")

	assert.NotEqual(t, hash1, hash2)
	assert.Len(t, hash1, 12)
	assert.Len(t, hash2, 12)
}

func Test_workloadArgumentString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple command",
			input:    "bash",
			expected: "bash",
		},
		{
			name:     "command with whitespace",
			input:    " bash ",
			expected: "bash",
		},
		{
			name:     "multipart script extracts first part",
			input:    "bash -c 'echo hello'",
			expected: "bash",
		},
		{
			name:     "flag without value is skipped",
			input:    "-v",
			expected: "",
		},
		{
			name:     "flag with value extracts flag name",
			input:    "--config=/path/to/config",
			expected: "--config",
		},
		{
			name:     "unix path extracts basename",
			input:    "/usr/local/bin/node",
			expected: "node",
		},
		{
			name:     "windows path extracts basename",
			input:    "c:\\windows\\system32\\cmd.exe",
			expected: "cmd.exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := workloadArgumentString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_idForImageReference(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid sha256 reference with full digest",
			input:    "registry.io/image@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			expected: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		{
			name:     "reference without digest returns empty",
			input:    "registry.io/image:latest",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := idForImageReference(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_matchingSpecIndex(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		spec          []corev1.Container
		hint          int
		expected      int
	}{
		{
			name:          "hint matches",
			containerName: "app",
			spec: []corev1.Container{
				{Name: "init"},
				{Name: "app"},
				{Name: "sidecar"},
			},
			hint:     1,
			expected: 1,
		},
		{
			name:          "hint doesn't match, find in list",
			containerName: "sidecar",
			spec: []corev1.Container{
				{Name: "init"},
				{Name: "app"},
				{Name: "sidecar"},
			},
			hint:     0,
			expected: 2,
		},
		{
			name:          "not found returns -1",
			containerName: "missing",
			spec: []corev1.Container{
				{Name: "app"},
			},
			hint:     0,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchingSpecIndex(tt.containerName, tt.spec, tt.hint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_workloadPodShapeIndex(t *testing.T) {
	tests := []struct {
		name     string
		shapes   []workloadPodShape
		shape    workloadPodShape
		expected int
	}{
		{
			name: "matching shape found",
			shapes: []workloadPodShape{
				{
					Containers: []workloadContainerShape{
						{ImageID: "sha256:abc"},
					},
				},
				{
					Containers: []workloadContainerShape{
						{ImageID: "sha256:def"},
					},
				},
			},
			shape: workloadPodShape{
				Containers: []workloadContainerShape{
					{ImageID: "sha256:def"},
				},
			},
			expected: 1,
		},
		{
			name: "no matching shape returns -1",
			shapes: []workloadPodShape{
				{
					Containers: []workloadContainerShape{
						{ImageID: "sha256:abc"},
					},
				},
			},
			shape: workloadPodShape{
				Containers: []workloadContainerShape{
					{ImageID: "sha256:xyz"},
				},
			},
			expected: -1,
		},
		{
			name: "matching shape with init containers",
			shapes: []workloadPodShape{
				{
					InitContainers: []workloadContainerShape{
						{ImageID: "sha256:init"},
					},
					Containers: []workloadContainerShape{
						{ImageID: "sha256:app"},
					},
				},
			},
			shape: workloadPodShape{
				InitContainers: []workloadContainerShape{
					{ImageID: "sha256:init"},
				},
				Containers: []workloadContainerShape{
					{ImageID: "sha256:app"},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := workloadPodShapeIndex(tt.shapes, tt.shape)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_workloadImageCache(t *testing.T) {
	workloadImageResize(10)

	image := workloadImage{
		LayerIDs: []string{"layer1", "layer2"},
	}
	workloadImageAdd("sha256:test", image)

	retrieved, ok := workloadImageGet("sha256:test")
	assert.True(t, ok)
	assert.Equal(t, image.LayerIDs, retrieved.LayerIDs)

	_, ok = workloadImageGet("sha256:nonexistent")
	assert.False(t, ok)
}

func Test_calculatePodShape(t *testing.T) {
	h := sha256.New()

	// Valid SHA256 digests (64 hex characters)
	validDigest1 := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	validDigest2 := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expectOk bool
	}{
		{
			name: "valid pod with containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:    "app",
							Image:   "registry.io/app@sha256:" + validDigest1,
							Command: []string{"/bin/sh"},
							Args:    []string{"-c", "start"},
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:    "app",
							ImageID: "docker-pullable://registry.io/app@sha256:" + validDigest1,
						},
					},
				},
			},
			expectOk: true,
		},
		{
			name: "pod with init and regular containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					InitContainers: []corev1.Container{
						{
							Name:  "init",
							Image: "registry.io/init@sha256:" + validDigest2,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "registry.io/app@sha256:" + validDigest1,
						},
					},
				},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							Name:    "init",
							ImageID: "docker-pullable://registry.io/init@sha256:" + validDigest2,
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:    "app",
							ImageID: "docker-pullable://registry.io/app@sha256:" + validDigest1,
						},
					},
				},
			},
			expectOk: true,
		},
		{
			name: "pod with missing image ID",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "registry.io/app:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:    "app",
							ImageID: "",
						},
					},
				},
			},
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shape, ok := calculatePodShape(h, tt.pod)
			assert.Equal(t, tt.expectOk, ok)
			if ok {
				assert.NotNil(t, shape.Containers)
				if tt.pod.Spec.RestartPolicy == corev1.RestartPolicyAlways {
					assert.True(t, shape.RestartsAlways)
				} else {
					assert.False(t, shape.RestartsAlways)
				}
			}
		})
	}
}

func Test_containerIDsByNode_addPodContainers(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected containerIDsByNode
	}{
		{
			name: "pod in Running phase adds container IDs",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "node-1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:        "app",
							ContainerID: "cri-o://abc123",
							State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			expected: containerIDsByNode{
				"node-1": []string{"cri-o://abc123"},
			},
		},
		{
			name: "pod in non-Running phase is skipped",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "node-1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:        "app",
							ContainerID: "cri-o://abc123",
						},
					},
				},
			},
			expected: containerIDsByNode{},
		},
		{
			name: "terminated containers are skipped",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "node-1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:        "app",
							ContainerID: "cri-o://abc123",
							State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
						{
							Name:        "sidecar",
							ContainerID: "cri-o://def456",
							State:       corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
						},
					},
				},
			},
			expected: containerIDsByNode{
				"node-1": []string{"cri-o://abc123"},
			},
		},
		{
			name: "running and waiting containers are included",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "node-1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:        "app",
							ContainerID: "cri-o://abc123",
							State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
						{
							Name:        "init",
							ContainerID: "cri-o://def456",
							State:       corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}},
						},
					},
				},
			},
			expected: containerIDsByNode{
				"node-1": []string{"cri-o://abc123", "cri-o://def456"},
			},
		},
		{
			name: "pod with missing container IDs handled gracefully",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "node-1",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:        "app",
							ContainerID: "",
							State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			expected: containerIDsByNode{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := make(containerIDsByNode)
			c.addPodContainers(tt.pod)
			assert.Equal(t, tt.expected, c)
		})
	}
}

func Test_containerIDsByNode_multiplePods(t *testing.T) {
	c := make(containerIDsByNode)

	// Add pods on same node - should accumulate IDs
	pod1 := &corev1.Pod{
		Spec: corev1.PodSpec{NodeName: "node-1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app1", ContainerID: "cri-o://abc123", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	pod2 := &corev1.Pod{
		Spec: corev1.PodSpec{NodeName: "node-1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app2", ContainerID: "cri-o://def456", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	// Pod on different node
	pod3 := &corev1.Pod{
		Spec: corev1.PodSpec{NodeName: "node-2"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app3", ContainerID: "cri-o://ghi789", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}

	c.addPodContainers(pod1)
	c.addPodContainers(pod2)
	c.addPodContainers(pod3)

	assert.Equal(t, 2, len(c))
	assert.Equal(t, []string{"cri-o://abc123", "cri-o://def456"}, c["node-1"])
	assert.Equal(t, []string{"cri-o://ghi789"}, c["node-2"])
}

func Test_mergeRuntimeInfoIntoShapes(t *testing.T) {
	tests := []struct {
		name         string
		info         workloadPods
		runtimeInfos workloadRuntimes
		checkResult  func(t *testing.T, info *workloadPods)
	}{
		{
			name: "merges runtime info into matching containers",
			info: workloadPods{
				Namespaces: map[string]workloadNamespacePods{
					"ns-hash": {
						Shapes: []workloadPodShape{
							{
								Containers: []workloadContainerShape{
									{
										ImageID: "sha256:abc",
										runtimeKey: containerInfo{
											namespace:   "test-ns",
											pod:         "test-pod",
											containerID: "cri-o://container-1",
										},
									},
								},
							},
						},
					},
				},
			},
			runtimeInfos: workloadRuntimes{
				{namespace: "test-ns", pod: "test-pod", containerID: "cri-o://container-1"}: {
					Os:   "rhel",
					Kind: "java",
				},
			},
			checkResult: func(t *testing.T, info *workloadPods) {
				container := &info.Namespaces["ns-hash"].Shapes[0].Containers[0]
				assert.NotNil(t, container.RuntimeInfo)
				assert.Equal(t, "rhel", container.RuntimeInfo.Os)
				assert.Equal(t, "java", container.RuntimeInfo.Kind)
			},
		},
		{
			name: "no runtime info for non-matching containers",
			info: workloadPods{
				Namespaces: map[string]workloadNamespacePods{
					"ns-hash": {
						Shapes: []workloadPodShape{
							{
								Containers: []workloadContainerShape{
									{
										ImageID: "sha256:abc",
										runtimeKey: containerInfo{
											namespace:   "test-ns",
											pod:         "test-pod",
											containerID: "cri-o://container-1",
										},
									},
								},
							},
						},
					},
				},
			},
			runtimeInfos: workloadRuntimes{
				{namespace: "other-ns", pod: "other-pod", containerID: "cri-o://other"}: {
					Os:   "rhel",
					Kind: "java",
				},
			},
			checkResult: func(t *testing.T, info *workloadPods) {
				container := &info.Namespaces["ns-hash"].Shapes[0].Containers[0]
				assert.Nil(t, container.RuntimeInfo)
			},
		},
		{
			name: "merges runtime info into init containers",
			info: workloadPods{
				Namespaces: map[string]workloadNamespacePods{
					"ns-hash": {
						Shapes: []workloadPodShape{
							{
								InitContainers: []workloadContainerShape{
									{
										ImageID: "sha256:init",
										runtimeKey: containerInfo{
											namespace:   "test-ns",
											pod:         "test-pod",
											containerID: "cri-o://init-container",
										},
									},
								},
								Containers: []workloadContainerShape{
									{
										ImageID: "sha256:app",
										runtimeKey: containerInfo{
											namespace:   "test-ns",
											pod:         "test-pod",
											containerID: "cri-o://app-container",
										},
									},
								},
							},
						},
					},
				},
			},
			runtimeInfos: workloadRuntimes{
				{namespace: "test-ns", pod: "test-pod", containerID: "cri-o://init-container"}: {
					Os: "ubuntu",
				},
				{namespace: "test-ns", pod: "test-pod", containerID: "cri-o://app-container"}: {
					Os:   "rhel",
					Kind: "nodejs",
				},
			},
			checkResult: func(t *testing.T, info *workloadPods) {
				initContainer := &info.Namespaces["ns-hash"].Shapes[0].InitContainers[0]
				assert.Nil(t, initContainer.RuntimeInfo)

				appContainer := &info.Namespaces["ns-hash"].Shapes[0].Containers[0]
				assert.NotNil(t, appContainer.RuntimeInfo)
				assert.Equal(t, "rhel", appContainer.RuntimeInfo.Os)
				assert.Equal(t, "nodejs", appContainer.RuntimeInfo.Kind)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeRuntimeInfoIntoShapes(&tt.info, tt.runtimeInfos)
			tt.checkResult(t, &tt.info)
		})
	}
}
