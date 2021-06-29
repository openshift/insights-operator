package clusterconfig

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherClusterProxy fetches the cluster Proxy - the Proxy with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/proxy.go#L30
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#proxy-v1-config-openshift-io
//
// * Location in archive: config/proxy/
// * See: docs/insights-archive-sample/config/proxy
// * Id in config: proxies
func (g *Gatherer) GatherClusterProxy(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterProxy(ctx, gatherConfigClient)
}

func gatherClusterProxy(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.Proxies().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{Name: "config/proxy", Item: record.ResourceMarshaller{Resource: anonymizeProxy(config)}}}, nil
}

func anonymizeProxy(proxy *configv1.Proxy) *configv1.Proxy {
	proxy.Spec.HTTPProxy = anonymize.URLCSV(proxy.Spec.HTTPProxy)
	proxy.Spec.HTTPSProxy = anonymize.URLCSV(proxy.Spec.HTTPSProxy)
	proxy.Spec.NoProxy = anonymize.URLCSV(proxy.Spec.NoProxy)
	proxy.Spec.ReadinessEndpoints = anonymize.URLSlice(proxy.Spec.ReadinessEndpoints)
	proxy.Status.HTTPProxy = anonymize.URLCSV(proxy.Status.HTTPProxy)
	proxy.Status.HTTPSProxy = anonymize.URLCSV(proxy.Status.HTTPSProxy)
	proxy.Status.NoProxy = anonymize.URLCSV(proxy.Status.NoProxy)
	return proxy
}
