package periodic

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift/insights-operator/pkg/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiWatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"
	"k8s.io/klog/v2"
)

// JobController type responsible for
// creating a new gathering jobs
type JobController struct {
	kubeClient kubernetes.Interface
}

func NewJobController(kubeClient kubernetes.Interface) *JobController {
	return &JobController{
		kubeClient: kubeClient,
	}
}

// CreateGathererJob creates a new Kubernetes Job with provided image, volume mount path used for storing data archives and name
// derived from the provided data gather name
//
//nolint:funlen
func (j *JobController) CreateGathererJob(
	ctx context.Context, dataGatherName, image string, dataReporting *config.DataReporting,
) (*batchv1.Job, error) {
	volumeSource := j.createVolumeSource(ctx, dataReporting.PersistentVolumeClaimName)

	gj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataGatherName,
			Namespace: insightsNamespace,
			Annotations: map[string]string{
				"openshift.io/required-scc": "restricted-v2",
			},
		},
		Spec: batchv1.JobSpec{
			// backoff limit is 0 - we dont' want to restart the gathering immediately in case of failure
			BackoffLimit: new(int32),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					PriorityClassName:  "system-cluster-critical",
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "operator",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &trueB,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name:         "archives-path",
							VolumeSource: volumeSource,
						},
						{
							Name: serviceCABundle,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: serviceCABundle,
									},
									Optional: &trueB,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "insights-gathering",
							Image: image,
							Args:  []string{"gather-and-upload", "-v=4", "--config=/etc/insights-operator/server.yaml"},
							Env: []corev1.EnvVar{
								{
									Name:  "DATAGATHER_NAME",
									Value: dataGatherName,
								},
								{
									Name:  "RELEASE_VERSION",
									Value: os.Getenv("RELEASE_VERSION"),
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("70Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: falseB,
								Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "archives-path",
									MountPath: dataReporting.StoragePath,
								},
								{
									Name:      serviceCABundle,
									MountPath: serviceCABundlePath,
								},
							},
						},
						{
							Name:    "cleanup-job",
							Image:   "registry.redhat.io/ubi8/ubi-minimal:latest",
							Command: []string{"sh", "-c"},
							Args: []string{`
								echo "Starting archives cleanup"

								ARCHIVES_COUNT=$(ls -p "$ARCHIVES_PATH" | grep -v / | wc -l)
								if [ $ARCHIVES_COUNT -lt 5 ]; then
									echo "No cleanup needed"
									exit 0
								fi

								FILE_TO_DELETE=$(ls -pt "$ARCHIVES_PATH" | grep -v / | tail -n 1)
								echo "Deleting $FILE_TO_DELETE"
								rm -f "$ARCHIVES_PATH/$FILE_TO_DELETE"

								echo "Archives cleanup finished"
							`},
							Env: []corev1.EnvVar{
								{
									Name:  "ARCHIVES_PATH",
									Value: dataReporting.StoragePath,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "archives-path",
									MountPath: dataReporting.StoragePath,
								},
							},
						},
					},
				},
			},
		},
	}

	return j.kubeClient.BatchV1().Jobs(insightsNamespace).Create(ctx, gj, metav1.CreateOptions{})
}

// WaitForJobCompletion listen the Kubernetes events to check if job finished.
func (j *JobController) WaitForJobCompletion(ctx context.Context, job *batchv1.Job) error {
	watcherFnc := func(_ metav1.ListOptions) (apiWatch.Interface, error) {
		return j.kubeClient.BatchV1().Jobs(insightsNamespace).
			Watch(ctx, metav1.ListOptions{FieldSelector: fmt.Sprintf("metadata.name=%s", job.Name)})
	}

	retryWatcher, err := watch.NewRetryWatcher(job.ResourceVersion, &cache.ListWatch{WatchFunc: watcherFnc})
	if err != nil {
		return err
	}

	defer retryWatcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-retryWatcher.ResultChan():
			if !ok {
				return fmt.Errorf("watcher channel was closed unexpectedly")
			}

			if event.Type == apiWatch.Deleted {
				return nil
			}

			if event.Type != apiWatch.Modified {
				continue
			}

			job, ok := event.Object.(*batchv1.Job)
			if !ok {
				return fmt.Errorf("failed to cast job event: %v", event.Object)
			}
			if job.Status.Succeeded > 0 {
				return nil
			}
			if job.Status.Failed > 0 {
				return fmt.Errorf("job %s failed", job.Name)
			}
		}
	}
}

func (j *JobController) createVolumeSource(ctx context.Context, persistentVolumeClaimName string) corev1.VolumeSource {
	pvc, err := j.kubeClient.CoreV1().PersistentVolumeClaims(insightsNamespace).Get(ctx, persistentVolumeClaimName, metav1.GetOptions{})
	if err == nil && pvc != nil {
		klog.Infof("Creating volume source with PersistentVolumeClaimName: %s", persistentVolumeClaimName)
		return corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: persistentVolumeClaimName,
			},
		}
	}

	klog.Infof("Unable to get PersistentVolumeClaim: %v", err)
	klog.Info("Creating volume source with EmptyDir")
	return corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
}
