// nolint: dupl
package clusterconfig

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterNetwork Collects the cluster Network - the Network with name cluster.
//
// ### API Reference
// - https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/network.go#L50
// - https://docs.openshift.com/container-platform/4.3/rest_api/index.html#network-v1-config-openshift-io
//
// ### Sample data
// - docs/insights-archive-sample/config/network.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.2.0  | config/network.json 					                    |
//
// ### Config ID
// `clusterconfig/networks`
//
// ### Released version
// - 4.2.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherClusterNetwork(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterNetwork(ctx, gatherConfigClient)
}

func gatherClusterNetwork(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.Networks().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	return []record.Record{{Name: "config/network", Item: record.ResourceMarshaller{Resource: config}}}, nil
}
