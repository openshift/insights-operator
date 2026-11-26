package util

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// HasDefaultStorageClass reports whether the cluster has a StorageClass
// marked as default (storageclass.kubernetes.io/is-default-class: "true").
// Without one, a PVC with no StorageClassName set stays Pending forever, so
// callers should skip rather than let the test time out.
func HasDefaultStorageClass(ctx context.Context, kubeClient kubernetes.Interface) (bool, error) {
	classes, err := kubeClient.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list storage classes: %w", err)
	}

	for _, class := range classes.Items {
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return true, nil
		}
	}
	return false, nil
}

// CreatePVC creates a PersistentVolumeClaim with the given name and size in
// InsightsNamespace. It does not wait for the PVC to become Bound: with a
// WaitForFirstConsumer storage class (the common default on AWS/GCP/Azure),
// the PV is not provisioned until a pod actually mounts the PVC, so blocking
// here would deadlock -- the DataGather's gathering pod is what triggers
// binding. Use WaitForPVCBound separately once something is expected to
// consume the PVC.
func CreatePVC(
	ctx context.Context, kubeClient kubernetes.Interface, name, size string,
) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: InsightsNamespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}

	created, err := kubeClient.CoreV1().PersistentVolumeClaims(InsightsNamespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create PVC %s/%s: %w", InsightsNamespace, name, err)
	}

	return created, nil
}

// WaitForPVCBound polls the named PVC until it becomes Bound. Call this after
// something that mounts the PVC (e.g. the DataGather's gathering pod) has
// been created, since a WaitForFirstConsumer storage class will not bind the
// PVC before then.
func WaitForPVCBound(
	ctx context.Context, kubeClient kubernetes.Interface, name string, timeout time.Duration,
) (*corev1.PersistentVolumeClaim, error) {
	var latest *corev1.PersistentVolumeClaim
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pvc, err := kubeClient.CoreV1().PersistentVolumeClaims(InsightsNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		latest = pvc
		return pvc.Status.Phase == corev1.ClaimBound, nil
	})
	if err != nil {
		return latest, fmt.Errorf("PVC %s/%s did not become Bound: %w", InsightsNamespace, name, err)
	}

	return latest, nil
}

// DeletePVC deletes the named PVC from InsightsNamespace, ignoring a
// not-found error.
func DeletePVC(ctx context.Context, kubeClient kubernetes.Interface, name string) error {
	err := kubeClient.CoreV1().PersistentVolumeClaims(InsightsNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete PVC %s/%s: %w", InsightsNamespace, name, err)
	}
	return nil
}
