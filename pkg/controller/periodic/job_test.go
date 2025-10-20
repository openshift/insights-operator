package periodic

import (
	"context"
	"testing"

	insightsv1 "github.com/openshift/api/insights/v1"
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
		name          string
		dataGather    *insightsv1.DataGather
		imageName     string
		dataReporting config.DataReporting
	}{
		{
			name: "Basic gathering job creation without PVC storage",
			dataGather: &insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "custom-gather-test-empty",
				},
			},
			imageName: "test.io/test/insights-image",
			dataReporting: config.DataReporting{
				StoragePath: storagePath,
			},
		},
		{
			name: "Basic gathering with PVC storage",
			dataGather: &insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "custom-gather-test-pvc",
				},
				Spec: insightsv1.DataGatherSpec{
					Storage: insightsv1.Storage{
						Type: insightsv1.StorageTypePersistentVolume,
						PersistentVolume: insightsv1.PersistentVolumeConfig{
							Claim: insightsv1.PersistentVolumeClaimReference{
								Name: insightsPVCName,
							},
						},
					},
				},
			},
			imageName: "test.io/test/insights-image",
			dataReporting: config.DataReporting{
				StoragePath: storagePath,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jc := NewJobController(kube)

			createdJob, err := jc.CreateGathererJob(context.Background(), tt.imageName, &tt.dataReporting, tt.dataGather)
			assert.NoError(t, err)
			assert.Equal(t, tt.dataGather.Name, createdJob.Name)
			assert.Equal(t, tt.imageName, createdJob.Spec.Template.Spec.Containers[0].Image)

			if tt.dataGather.Spec.Storage.Type != insightsv1.StorageTypePersistentVolume {
				// EmptyDir is used when no storage is specified
				assert.NotNil(t, createdJob.Spec.Template.Spec.Volumes[0].EmptyDir)
				assert.Nil(t, createdJob.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim)
			} else {
				assert.NotNil(t, createdJob.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim)
				assert.Nil(t, createdJob.Spec.Template.Spec.Volumes[0].EmptyDir)
				assert.Equal(t, insightsPVCName, createdJob.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)
			}

			// we mount to volumes
			assert.Len(t, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts, 2)
			assert.Equal(t, tt.dataReporting.StoragePath, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)
		})
	}
}

func TestCreateEnvVar(t *testing.T) {
	tests := []struct {
		name        string
		expectedEnv []v1.EnvVar
	}{
		{
			name: "without-proxy-configuration",
			expectedEnv: []v1.EnvVar{
				{
					Name:  "RELEASE_VERSION",
					Value: "test-version",
				},
				{
					Name:  "DATAGATHER_NAME",
					Value: "without-proxy-configuration",
				},
			},
		},
		{
			name: "with-proxy-configuration",
			expectedEnv: []v1.EnvVar{
				{
					Name:  "HTTP_PROXY",
					Value: "http://test-proxy.com",
				},
				{
					Name:  "HTTPS_PROXY",
					Value: "https://test-proxy.com",
				},
				{
					Name:  "NO_PROXY",
					Value: "http://test-no-proxy.com",
				},
				{
					Name:  "RELEASE_VERSION",
					Value: "test-version",
				},
				{
					Name:  "DATAGATHER_NAME",
					Value: "with-proxy-configuration",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, env := range tt.expectedEnv {
				t.Setenv(env.Name, env.Value)
			}

			assert.Equal(t, tt.expectedEnv, createEnvVar(tt.name))
		})
	}
}
