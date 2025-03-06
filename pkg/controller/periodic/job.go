package periodic

import (
	"context"
	"fmt"
	"os"

	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
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
	"k8s.io/utils/ptr"
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
func (j *JobController) CreateGathererJob(
	ctx context.Context, dataGatherName, image string, dataReporting *config.DataReporting, storage *insightsv1alpha1.Storage,
) (*batchv1.Job, error) {
	volumeSource := j.createVolumeSource(ctx, storage)
	volumeMounts := j.createVolumeMounts(dataReporting.StoragePath, storage)

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
						RunAsNonRoot: ptr.To(true),
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
									Optional: ptr.To(true),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "insights-gathering",
							Image: image,
							Args:  []string{"gather-and-upload", "-v=4", "--config=/etc/insights-operator/server.yaml", "--storagePath", volumeMounts[0].MountPath},
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
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							},
							VolumeMounts: volumeMounts,
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

func (j *JobController) createVolumeSource(ctx context.Context, storage *insightsv1alpha1.Storage) corev1.VolumeSource {
	if storage == nil {
		klog.Info("Creating volume source with EmptyDir, no storageSpec provided")
		return corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}

	if storage.Type == insightsv1alpha1.StorageTypePersistentVolume {
		// Validate if the PVC exists
		persistentVolumeClaimName := string(storage.PersistentVolume.Claim.Name)
		pvc, err := j.kubeClient.CoreV1().PersistentVolumeClaims(insightsNamespace).Get(ctx, persistentVolumeClaimName, metav1.GetOptions{})
		if err != nil {
			klog.Error(err, " Failed to get PersistentVolumeClaim with name ", persistentVolumeClaimName)
		} else if pvc != nil {
			klog.Infof("Creating volume source with PersistentVolumeClaimName: %s", persistentVolumeClaimName)
			return corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: persistentVolumeClaimName,
				},
			}
		}
	}

	klog.Info("Creating volume source with EmptyDir")
	return corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
}

func (j *JobController) createVolumeMounts(storagePath string, storage *insightsv1alpha1.Storage) []corev1.VolumeMount {
	volumeMount := []corev1.VolumeMount{
		{
			Name:      "archives-path",
			MountPath: storagePath,
		},
		{
			Name:      serviceCABundle,
			MountPath: serviceCABundlePath,
		},
	}

	if storage == nil || storage.Type != insightsv1alpha1.StorageTypePersistentVolume {
		return volumeMount
	}

	// If the PVC has a mountPath, use it
	if mountPath := storage.PersistentVolume.MountPath; mountPath != "" {
		volumeMount[0].MountPath = mountPath
	}

	return volumeMount
}
