package periodic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestCreateGathererJob(t *testing.T) {
	tests := []struct {
		name            string
		dataGatherName  string
		imageName       string
		volumeMountPath string
	}{
		{
			name:            "Basic gathering job creation",
			dataGatherName:  "custom-gather-xyz",
			imageName:       "test.io/test/insights-image",
			volumeMountPath: "/var/lib/test-io/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := kubefake.NewSimpleClientset()
			jc := NewJobController(cs)
			createdJob, err := jc.CreateGathererJob(context.Background(), tt.dataGatherName, tt.imageName, tt.volumeMountPath)
			assert.NoError(t, err)
			assert.Equal(t, tt.dataGatherName, createdJob.Name)
			assert.Len(t, createdJob.Spec.Template.Spec.Containers, 1)
			assert.Equal(t, tt.imageName, createdJob.Spec.Template.Spec.Containers[0].Image)
			// we mount to volumes
			assert.Len(t, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts, 2)
			assert.Equal(t, tt.volumeMountPath, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)
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
