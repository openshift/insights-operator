package util

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
)

// archiveReaderImage is used for the short-lived helper pod that reads the
// archive off the PVC. It only needs a shell, "ls" and "cat".
const archiveReaderImage = "quay.io/openshifttest/busybox"

const archiveReaderContainerName = "archive-reader"

// CreateArchiveReaderPod creates a short-lived pod in InsightsNamespace that
// mounts the named PVC read-only at mountPath, so the test can exec into it
// to read the recorded archive.
//
// gatheringJob must be the Job returned by
// WaitForGatheringJobComplete for the DataGather that owns pvcName. The PVC
// is ReadWriteOnce, and the gathering pod holds it mounted until that Job
// finishes; requiring the Job here makes it impossible to call this before
// the gathering pod has released the volume.
func CreateArchiveReaderPod(
	ctx context.Context, kubeClient kubernetes.Interface, cfg *rest.Config, gatheringJob *batchv1.Job,
	name, pvcName, mountPath string, readyTimeout time.Duration,
) (*corev1.Pod, error) {
	if !jobComplete(gatheringJob) {
		return nil, fmt.Errorf(
			"archive reader pod %s/%s requires a completed gathering job, got: %#v", InsightsNamespace, name, gatheringJob,
		)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: InsightsNamespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    archiveReaderContainerName,
					Image:   archiveReaderImage,
					Command: []string{"sh", "-c", "sleep 3600"},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "archive", MountPath: mountPath, ReadOnly: true},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "archive",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
							ReadOnly:  true,
						},
					},
				},
			},
		},
	}

	if _, err := kubeClient.CoreV1().Pods(InsightsNamespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("failed to create archive reader pod %s/%s: %w", InsightsNamespace, name, err)
	}

	var latest *corev1.Pod
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, readyTimeout, true, func(ctx context.Context) (bool, error) {
		p, err := kubeClient.CoreV1().Pods(InsightsNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		latest = p
		return p.Status.Phase == corev1.PodRunning, nil
	})
	if err != nil {
		return latest, fmt.Errorf("archive reader pod %s/%s did not become Running: %w", InsightsNamespace, name, err)
	}

	// Phase turning Running only means the container process started; it
	// does not guarantee the kubelet's exec/attach serving path is up yet.
	// Poll with a real (cheap) exec until the pod actually accepts one,
	// instead of letting the first real exec from a caller race it.
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, readyTimeout, true, func(ctx context.Context) (bool, error) {
		_, err := execInPod(ctx, kubeClient, cfg, name, []string{"true"})
		return err == nil, nil
	})
	if err != nil {
		return latest, fmt.Errorf("archive reader pod %s/%s did not become exec-ready: %w", InsightsNamespace, name, err)
	}

	return latest, nil
}

// execExchangeTimeout bounds a single exec round-trip (e.g. "ls") that isn't
// expected to move much data. ReadFileFromPod uses a larger, caller-supplied
// timeout instead, since archive sizes vary.
const execExchangeTimeout = 30 * time.Second

// jobComplete reports whether job has a True JobComplete condition, i.e. it
// is the same check WaitForGatheringJobComplete polls for.
func jobComplete(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// DeleteArchiveReaderPod deletes the named helper pod, ignoring a not-found
// error.
func DeleteArchiveReaderPod(ctx context.Context, kubeClient kubernetes.Interface, name string) error {
	err := kubeClient.CoreV1().Pods(InsightsNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete archive reader pod %s/%s: %w", InsightsNamespace, name, err)
	}
	return nil
}

// FindNewestArchive lists mountPath inside the named pod and returns the name
// of the most recent "insights-*.tar.gz" file found there.
func FindNewestArchive(
	ctx context.Context, kubeClient kubernetes.Interface, cfg *rest.Config, podName, mountPath string,
) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, execExchangeTimeout)
	defer cancel()

	// -t sorts by modification time, newest first; -1 forces one name per
	// line. The loop below then returns the first insights-*.tar.gz match,
	// i.e. the most recently modified archive.
	out, err := execInPod(execCtx, kubeClient, cfg, podName, []string{"ls", "-1", "-t", mountPath})
	if err != nil {
		return "", fmt.Errorf("failed to list %s in pod %s/%s: %w", mountPath, InsightsNamespace, podName, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, "insights-") && strings.HasSuffix(name, ".tar.gz") {
			return name, nil
		}
	}

	return "", fmt.Errorf("no insights-*.tar.gz archive found in %s", mountPath)
}

// ReadFileFromPod exec's "cat" on the given path inside the named pod and
// returns its raw contents. timeout should account for the file's expected
// size: a real insights archive can be several MB, which can take longer
// than a short fixed timeout to stream over the exec connection.
func ReadFileFromPod(
	ctx context.Context, kubeClient kubernetes.Interface, cfg *rest.Config, podName, path string, timeout time.Duration,
) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := execInPod(execCtx, kubeClient, cfg, podName, []string{"cat", path})
	if err != nil {
		return nil, fmt.Errorf("failed to read %s from pod %s/%s: %w", path, InsightsNamespace, podName, err)
	}
	return out, nil
}

// execInPod runs command in the archive reader container, bounded by ctx
// (the caller is expected to attach a timeout appropriate to the command).
func execInPod(
	ctx context.Context, kubeClient kubernetes.Interface, cfg *rest.Config, podName string, command []string,
) ([]byte, error) {
	req := kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(InsightsNamespace).
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: archiveReaderContainerName,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SPDY executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		return nil, fmt.Errorf("exec %v failed: %w (stderr: %s)", command, err, stderr.String())
	}

	return stdout.Bytes(), nil
}
