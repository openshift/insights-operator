package periodic

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
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
func (j *JobController) CreateGathererJob(ctx context.Context, dataGatherName, image, archiveVolumeMountPath string) (*batchv1.Job, error) {
	gj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataGatherName,
			Namespace: insightsNamespace,
		},
		Spec: batchv1.JobSpec{
			// backoff limit is 0 - we dont' want to restart the gathering immediately in case of failure
			BackoffLimit: new(int32),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
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
							Name: "archives-path",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
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
									MountPath: archiveVolumeMountPath,
								},
								{
									Name:      serviceCABundle,
									MountPath: serviceCABundlePath,
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

// WaitForJobCompletion polls the Kubernetes API every 20 seconds and checks if the job finished.
func (j *JobController) WaitForJobCompletion(ctx context.Context, job *batchv1.Job) error {
	return wait.PollUntilContextCancel(ctx, 20*time.Second, true, func(ctx context.Context) (done bool, err error) {
		j, err := j.kubeClient.BatchV1().Jobs(insightsNamespace).Get(ctx, job.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return false, err
		}
		if j.Status.Succeeded > 0 {
			return true, nil
		}
		if j.Status.Failed > 0 {
			return true, fmt.Errorf("job %s failed", job.Name)
		}
		return false, nil
	})
}
