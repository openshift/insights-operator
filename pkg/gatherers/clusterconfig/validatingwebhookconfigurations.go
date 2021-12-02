//nolint:dupl
package clusterconfig

import (
	"context"
	"fmt"

	admissionregistrationv1types "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	admissionregistrationv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherValidatingWebhookConfigurations collects ValidatingWebhookConfiguration resources
// Relevant OpenShift API docs:
//   - https://docs.openshift.com/container-platform/4.8/rest_api/extension_apis/validatingwebhookconfiguration-admissionregistration-k8s-io-v1.html
//
// * Location in archive: config/validatingwebhookconfigurations
// * Since versions:
//   * 4.10+
func (g *Gatherer) GatherValidatingWebhookConfigurations(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherValidatingWebhookConfigurations(ctx, gatherKubeClient.AdmissionregistrationV1())
}

func gatherValidatingWebhookConfigurations(
	ctx context.Context, client admissionregistrationv1.AdmissionregistrationV1Interface,
) ([]record.Record, []error) {
	validatingWebhookConfigurations, err := client.ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i := range validatingWebhookConfigurations.Items {
		validatingWebhookConfiguration := validatingWebhookConfigurations.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/validatingwebhookconfigurations/%v", validatingWebhookConfiguration.Name),
			Item: record.ResourceMarshaller{
				Resource: anonymizeValidatingWebhookConfiguration(&validatingWebhookConfiguration),
			},
		})
	}

	return records, nil
}

func anonymizeValidatingWebhookConfiguration(
	configuration *admissionregistrationv1types.ValidatingWebhookConfiguration,
) *admissionregistrationv1types.ValidatingWebhookConfiguration {
	for i := range configuration.Webhooks {
		webhook := &configuration.Webhooks[i]
		webhook.ClientConfig.CABundle = []byte(anonymize.String(string(webhook.ClientConfig.CABundle)))
	}

	return configuration
}
