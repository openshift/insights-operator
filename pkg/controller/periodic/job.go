package periodic

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type JobController struct {
	kubeClient *kubernetes.Clientset
}

func NewJobController(kubeClient *kubernetes.Clientset) *JobController {
	return &JobController{
		kubeClient: kubeClient,
	}
}

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
