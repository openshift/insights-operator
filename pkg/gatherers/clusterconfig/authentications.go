//nolint: dupl
package clusterconfig

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterAuthentication fetches the cluster Authentication - the Authentication with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/authentication.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#authentication-v1operator-openshift-io
//
// * Location in archive: config/authentication/
// * See: docs/insights-archive-sample/config/authentication
// * Id in config: authentication
func (g *Gatherer) GatherClusterAuthentication(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterAuthentication(ctx, gatherConfigClient)
}

func gatherClusterAuthentication(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.Authentications().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{Name: "config/authentication", Item: record.ResourceMarshaller{Resource: config}}}, nil
}
