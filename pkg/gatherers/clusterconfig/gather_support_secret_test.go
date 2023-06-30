package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/record"
)

func Test_gatherSupportSecret(t *testing.T) {
	kubeClient := kubefake.NewSimpleClientset()
	_, err := kubeClient.CoreV1().Secrets("openshift-config").Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "openshift-config",
			Name:      "support",
		},
		Data: map[string][]byte{
			"conditionalGathererEndpoint": []byte("http://localhost:8080/"),
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)
	configObserver := configobserver.New(config.Controller{}, kubeClient)
	gatherer := New(
		nil, nil, nil, nil, nil, configObserver,
	)

	records, errs := gatherer.GatherSupportSecret(context.TODO())
	assert.Empty(t, errs)
	assert.Len(t, records, 1)
	assert.Equal(t, record.Record{
		Name: "config/secrets/openshift-config/support/data",
		Item: record.JSONMarshaller{Object: map[string][]byte{
			"conditionalGathererEndpoint": []byte("http://localhost:8080/"),
		}},
	}, records[0])
}
