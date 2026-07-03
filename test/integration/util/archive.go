package util

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	insightsv1 "github.com/openshift/api/insights/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ReadArchiveFromPVC reads archive by mounting PVC to a test pod
func ReadArchiveFromPVC(ctx context.Context, pvcName, namespace string) ([]byte, error) {
	kubeClient := GetKubeClient()

	// Create test pod with PVC mounted
	podName := "test-archive-reader-" + RandomSuffix()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "reader",
					Image:   "registry.redhat.io/ubi8/ubi-minimal:latest",
					Command: []string{"sleep", "3600"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "archive-volume",
							MountPath: "/archive",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "archive-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err := kubeClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create reader pod: %w", err)
	}
	defer kubeClient.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})

	// Wait for pod to be running
	if err := waitForPodRunning(ctx, kubeClient, namespace, podName, 2*time.Minute); err != nil {
		return nil, fmt.Errorf("reader pod did not start: %w", err)
	}

	// Read archive from standardized path: /archive/insights-*.tar.gz
	archiveData, err := execInPod(ctx, kubeClient, namespace, podName, "reader",
		[]string{"sh", "-c", "cat /archive/*.tar.gz"})
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	return archiveData, nil
}

// ExtractFilesMatching extracts files from tar.gz matching pattern
func ExtractFilesMatching(archiveData []byte, pattern string) (map[string][]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	files := make(map[string][]byte)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		if strings.Contains(header.Name, pattern) {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %w", header.Name, err)
			}
			files[header.Name] = content
		}
	}

	return files, nil
}

// ListArchiveContents returns a list of all files in the tar.gz archive
func ListArchiveContents(archiveData []byte) ([]string, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var files []string

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		files = append(files, header.Name)
	}

	return files, nil
}

// CreateTestPVC creates a PVC for test purposes
func CreateTestPVC(ctx context.Context, name, namespace string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	kubeClient := GetKubeClient()
	created, err := kubeClient.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create PVC: %w", err)
	}

	return created, nil
}

// HasCondition checks if a DataGather has a specific condition with given status
func HasCondition(dg *insightsv1.DataGather, condType string, status metav1.ConditionStatus) bool {
	for _, cond := range dg.Status.Conditions {
		if cond.Type == condType && cond.Status == status {
			return true
		}
	}
	return false
}

// waitForPodRunning waits for a pod to be in Running phase using Kubernetes wait utilities
func waitForPodRunning(ctx context.Context, client kubernetes.Interface, namespace, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			// Continue polling on transient errors
			return false, nil
		}
		// Pod is ready when it's in Running phase
		return pod.Status.Phase == corev1.PodRunning, nil
	})
}

// execInPod executes a command in a pod and returns stdout
// Uses the standard Kubernetes remotecommand package with WebSocket (preferred over deprecated SPDY)
func execInPod(ctx context.Context, client kubernetes.Interface, namespace, pod, container string, command []string) ([]byte, error) {
	// Construct the exec request
	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	// Get REST config from our client initialization
	config := GetRestConfig()

	// Try WebSocket first (modern approach), fall back to SPDY if needed
	exec, err := remotecommand.NewWebSocketExecutor(config, "POST", req.URL().String())
	if err != nil {
		// Fallback to SPDY for older clusters
		exec, err = remotecommand.NewSPDYExecutor(config, "POST", req.URL())
		if err != nil {
			return nil, fmt.Errorf("failed to create executor: %w", err)
		}
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}
