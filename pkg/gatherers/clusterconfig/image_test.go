package clusterconfig

import (
	"context"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GatherClusterImage(t *testing.T) {
	cfg := configfake.NewSimpleClientset()
	testImage := &v1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: v1.ImageSpec{
			AdditionalTrustedCA: v1.ConfigMapNameReference{
				Name: "test-cm-reference",
			},
		},
		Status: v1.ImageStatus{
			InternalRegistryHostname: "test-registry-name",
		},
	}
	_, err := cfg.ConfigV1().Images().Create(context.Background(), testImage, metav1.CreateOptions{})
	assert.NoError(t, err, "unable to create fake image")
	records, errs := gatherClusterImage(context.Background(), cfg.ConfigV1())
	assert.Len(t, records, 1)
	assert.Len(t, errs, 0)

	item := records[0].Item
	clusterImage, ok := item.(record.ResourceMarshaller).Resource.(*v1.Image)
	assert.True(t, ok)
	assert.Equal(t, "test-cm-reference", clusterImage.Spec.AdditionalTrustedCA.Name)
	assert.Equal(t, "test-registry-name", clusterImage.Status.InternalRegistryHostname)
}
