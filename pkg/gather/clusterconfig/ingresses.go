package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterIngress fetches the cluster Ingress - the Ingress with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/ingress.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#ingress-v1-config-openshift-io
//
// Location in archive: config/ingress/
// See: docs/insights-archive-sample/config/ingress
// Id in config: ingress
func GatherClusterIngress(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherClusterIngress(g.ctx, gatherConfigClient)
	c <- gatherResult{records, errors}
}

func gatherClusterIngress(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.Ingresses().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{Name: "config/ingress", Item: IngressAnonymizer{config}}}, nil
}

// IngressAnonymizer implements serialization with marshalling
type IngressAnonymizer struct{ *configv1.Ingress }

// Marshal implements serialization of Ingres.Spec.Domain with anonymization
func (a IngressAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Ingress.Spec.Domain = anonymizeURL(a.Ingress.Spec.Domain)
	return runtime.Encode(openshiftSerializer, a.Ingress)
}

// GetExtension returns extension for anonymized ingress objects
func (a IngressAnonymizer) GetExtension() string {
	return "json"
}
