// nolint: dupl
package clusterconfig

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterFeatureGates Collects the cluster `FeatureGate` with cluster name.
//
// ### API Reference
// - https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/featuregate.go#L50
// - https://docs.openshift.com/container-platform/4.3/rest_api/index.html#featuregate-v1-config-openshift-io
//
// ### Sample data
// - docs/insights-archive-sample/config/featuregate.json
//
// ### Location in archive
// - `config/featuregate.json`
//
// ### Config ID
// `clusterconfig/feature_gates`
//
// ### Released version
// - 4.2.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherClusterFeatureGates(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterFeatureGates(ctx, gatherConfigClient)
}

func gatherClusterFeatureGates(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.FeatureGates().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{Name: "config/featuregate", Item: record.ResourceMarshaller{Resource: config}}}, nil
}
