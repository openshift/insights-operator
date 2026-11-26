package util

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	insightsv1 "github.com/openshift/api/insights/v1"
	insightsclientset "github.com/openshift/client-go/insights/clientset/versioned"
)

// DefaultPVCMountPath is the mount path set on the DataGather's
// PersistentVolume storage so the gathering pod's archive location is known
// ahead of time, rather than relying on the operator's internal default.
const DefaultPVCMountPath = "/var/lib/insights-operator"

// CreatePVCDataGather creates a cluster-scoped DataGather that stores its
// archive on the PVC named pvcName (which must already exist in
// InsightsNamespace).
func CreatePVCDataGather(
	ctx context.Context, insightsClient insightsclientset.Interface, pvcName string,
) (*insightsv1.DataGather, error) {
	dg := &insightsv1.DataGather{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-datagather-",
		},
		Spec: insightsv1.DataGatherSpec{
			Gatherers: insightsv1.Gatherers{
				Mode: insightsv1.GatheringModeAll,
			},
			Storage: insightsv1.Storage{
				Type: insightsv1.StorageTypePersistentVolume,
				PersistentVolume: insightsv1.PersistentVolumeConfig{
					Claim: insightsv1.PersistentVolumeClaimReference{
						Name: pvcName,
					},
					MountPath: DefaultPVCMountPath,
				},
			},
		},
	}

	created, err := insightsClient.InsightsV1().DataGathers().Create(ctx, dg, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create DataGather referencing PVC %s: %w", pvcName, err)
	}

	return created, nil
}

// DeleteDataGather deletes the named cluster-scoped DataGather, ignoring a
// not-found error.
func DeleteDataGather(ctx context.Context, insightsClient insightsclientset.Interface, name string) error {
	err := insightsClient.InsightsV1().DataGathers().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete DataGather %s: %w", name, err)
	}
	return nil
}
