package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	installertypes "github.com/openshift/installer/pkg/types"
	vsphere "github.com/openshift/installer/pkg/types/vsphere"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_gatherClusterConfigV1(t *testing.T) {
	coreClient := kubefake.NewSimpleClientset()

	_, err := coreClient.CoreV1().ConfigMaps("kube-system").Create(context.Background(), &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-config-v1",
		},
		Immutable: nil,
		Data: map[string]string{
			"install-config": "{}",
		},
		BinaryData: nil,
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	records, errs := gatherClusterConfigV1(context.Background(), coreClient.CoreV1())
	assert.Empty(t, errs)

	assert.Len(t, records, 1)
	assert.Equal(t, "config/configmaps/kube-system/cluster-config-v1/install-config", records[0].Name)

	data, err := records[0].Item.(ConfigMapAnonymizer).Marshal()
	assert.NoError(t, err)

	installConfig := `baseDomain: ""
metadata:
  creationTimestamp: null
platform: {}
pullSecret: ""
`

	assert.Equal(t, installConfig, string(data))
}

func TestAnonymizeInstallConfigVSphere(t *testing.T) {
	// Given
	testUsername, expectedUsername := "test", anonymize.String("test")
	testPassword, expectedPassword := "test", anonymize.String("test")
	testDatacenters, expectedDatacenters :=
		[]string{"test", "test2"}, []string{anonymize.String("test"), anonymize.String("test2")}

	givenIC := installertypes.InstallConfig{
		Platform: installertypes.Platform{
			VSphere: &vsphere.Platform{
				VCenters: []vsphere.VCenter{{
					Username: testUsername, Password: testPassword, Datacenters: testDatacenters},
				},
			},
		},
	}

	// Test
	result := anonymizeInstallConfig(&givenIC)

	// Assert
	assert.Equal(t, expectedUsername, result.VSphere.VCenters[0].Username)
	assert.Equal(t, expectedPassword, result.VSphere.VCenters[0].Password)
	assert.ElementsMatch(t, expectedDatacenters, result.VSphere.VCenters[0].Datacenters)
}
