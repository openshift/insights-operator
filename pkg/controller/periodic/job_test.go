package periodic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.Len(t, createdJob.Spec.Template.Spec.Containers, 2)
			assert.Equal(t, tt.imageName, createdJob.Spec.Template.Spec.Containers[0].Image)
			// we mount to volumes
			assert.Len(t, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts, 2)
			assert.Equal(t, tt.volumeMountPath, createdJob.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)
		})
	}
}
