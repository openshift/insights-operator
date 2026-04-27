package clusterconfig

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_gatherValidatingWebhookConfigurations(t *testing.T) {
	client := kubefake.NewClientset().AdmissionregistrationV1()
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

	// Verify marshaling works
	_, err = records[0].Item.Marshal()
	assert.NoError(t, err)

	// Verify the resource content by accessing it directly
	webhookConfig, ok := records[0].Item.(record.ResourceMarshaller).Resource.(*admissionregistrationv1.ValidatingWebhookConfiguration)
	assert.True(t, ok)
	assert.Equal(t, "webhook_config", webhookConfig.Name)
	assert.Len(t, webhookConfig.Webhooks, 2)
	assert.Equal(t, "webhook1", webhookConfig.Webhooks[0].Name)
	assert.Equal(t, "webhook2", webhookConfig.Webhooks[1].Name)
}
