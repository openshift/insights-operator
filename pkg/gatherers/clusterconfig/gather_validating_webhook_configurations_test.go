package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_gatherValidatingWebhookConfigurations(t *testing.T) {
	client := kubefake.NewSimpleClientset().AdmissionregistrationV1()
	_, err := client.ValidatingWebhookConfigurations().Create(context.TODO(), &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook_config",
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "webhook1",
			},
			{
				Name: "webhook2",
			},
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	records, errs := gatherValidatingWebhookConfigurations(context.TODO(), client)
	assert.Empty(t, errs)

	assert.Len(t, records, 1)
	assert.Equal(t, records[0].Name, "config/validatingwebhookconfigurations/webhook_config")

	configurationBytes, err := records[0].Item.Marshal()
	assert.NoError(t, err)

	assert.JSONEq(t, `{
		"metadata": { "name": "webhook_config" },
		"webhooks": [
			{ "name": "webhook1", "clientConfig": {}, "sideEffects": null, "admissionReviewVersions": null },
			{ "name": "webhook2", "clientConfig": {}, "sideEffects": null, "admissionReviewVersions": null }
		]
	}`, string(configurationBytes))
}
