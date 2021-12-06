package clusterconfig

import (
	"context"
	"testing"

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
	assert.Equal(t, "config/configmaps/kube-system/cluster-config-v1", records[0].Name)

	data, err := records[0].Item.Marshal(context.Background())
	assert.NoError(t, err)

	installConfig := `baseDomain: \"\"\nmetadata:\n  creationTimestamp: null\nplatform: {}\npullSecret: \"\"\n`

	assert.JSONEq(t, `{
		"metadata": {
			"name": "cluster-config-v1",
			"namespace": "kube-system",
			"creationTimestamp": null
		},
		"data": {
			"install-config": "`+installConfig+`"
		}
	}`, string(data))
}
