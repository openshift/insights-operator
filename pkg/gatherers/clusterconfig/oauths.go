// nolint: dupl
package clusterconfig

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterOAuth Collects the cluster OAuth - the OAuth with name cluster.
//
// ### API Reference
// - https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/oauth.go#L50
// - https://docs.openshift.com/container-platform/4.3/rest_api/index.html#oauth-v1-config-openshift-io
//
// ### Sample data
// - docs/insights-archive-sample/config/oauth.json
//
// ### Location in archive
// - `config/oauth.json`
//
// ### Config ID
// `clusterconfig/oauths`
//
// ### Released version
// - 4.2.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherClusterOAuth(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterOAuth(ctx, gatherConfigClient)
}

func gatherClusterOAuth(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.OAuths().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	return []record.Record{{Name: "config/oauth", Item: record.ResourceMarshaller{Resource: config}}}, nil
}
