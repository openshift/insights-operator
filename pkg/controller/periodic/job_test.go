package periodic

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/config"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

const (
	insightsPVCName = "test-pvc"
	storagePath     = "/var/lib/test-io/path"
)

func TestCreateGathererJob(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()

	_, err := coreClient.PersistentVolumeClaims(insightsNamespace).Create(context.TODO(), &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: insightsPVCName,
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	tests := []struct {
		name           string
		dataGatherName string
		imageName      string
		dataReporting  config.DataReporting
	}{
		{
			name:           "Basic gathering job creation without PVC storage",
			dataGatherName: "custom-gather-test-empty",
			imageName:      "test.io/test/insights-image",
			dataReporting: config.DataReporting{
				StoragePath: storagePath,
			},
		},
		{
			name:           "Basic gathering with PVC storage",
			dataGatherName: "custom-gather-test-pvc",
			imageName:      "test.io/test/insights-image",
			dataReporting: config.DataReporting{
				StoragePath:               storagePath,
				PersistentVolumeClaimName: insightsPVCName,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jc := NewJobController(kube)

			createdJob, err := jc.CreateGathererJob(context.Background(), tt.dataGatherName, tt.imageName, &tt.dataReporting)
			assert.NoError(t, err)
			assert.Equal(t, tt.dataGatherName, createdJob.Name)
			assert.Len(t, createdJob.Spec.Template.Spec.Containers, 2)
			assert.Equal(t, tt.imageName, createdJob.Spec.Template.Spec.Containers[0].Image)

			if tt.dataReporting.PersistentVolumeClaimName == "" {
				// EmptyDir is used when no PVC is specified
				assert.NotNil(t, createdJob.Spec.Template.Spec.Volumes[0].EmptyDir)
				assert.Nil(t, createdJob.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim)
			} else {
				assert.NotNil(t, createdJob.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim)
				assert.Nil(t, createdJob.Spec.Template.Spec.Volumes[0].EmptyDir)
				assert.Equal(t, "test-pvc", createdJob.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)
			}

			// we mount to volumes
			assert.Len(t, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts, 2)
			assert.Equal(t, tt.dataReporting.StoragePath, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)
		})
	}
}
