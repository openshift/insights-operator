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

// GatherMutatingWebhookConfigurations collects MutatingWebhookConfiguration resources
// Relevant OpenShift API docs:
//   - https://docs.openshift.com/container-platform/4.8/rest_api/extension_apis/mutatingwebhookconfiguration-admissionregistration-k8s-io-v1.html
//
// * Location in archive: config/mutatingwebhookconfigurations
// * Since versions:
//   * 4.10+
func (g *Gatherer) GatherMutatingWebhookConfigurations(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMutatingWebhookConfigurations(ctx, gatherKubeClient.AdmissionregistrationV1())
}

func gatherMutatingWebhookConfigurations(
	ctx context.Context, client admissionregistrationv1.AdmissionregistrationV1Interface,
) ([]record.Record, []error) {
	mutatingWebhookConfigurations, err := client.MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i := range mutatingWebhookConfigurations.Items {
		mutatingWebhookConfiguration := mutatingWebhookConfigurations.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/mutatingwebhookconfigurations/%v", mutatingWebhookConfiguration.Name),
			Item: record.ResourceMarshaller{
				Resource: anonymizeMutatingWebhookConfiguration(&mutatingWebhookConfiguration),
			},
		})
	}

	return records, nil
}

func anonymizeMutatingWebhookConfiguration(
	configuration *admissionregistrationv1types.MutatingWebhookConfiguration,
) *admissionregistrationv1types.MutatingWebhookConfiguration {
	for i := range configuration.Webhooks {
		webhook := &configuration.Webhooks[i]
		webhook.ClientConfig.CABundle = []byte(anonymize.String(string(webhook.ClientConfig.CABundle)))
	}

	return configuration
}
