// nolint: dupl
package clusterconfig

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatherClusterImages Collects cluster `images.config.openshift.io` resource definition.
//
// ### API Reference
// - https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/config_client.go#L72
// - https://docs.openshift.com/container-platform/latest/rest_api/config_apis/image-config-openshift-io-v1.html#image-config-openshift-io-v1
//
// ### Sample data
// - docs/insights-archive-sample/config/image.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | > 4.11.0  | config/image.json					                        |
//
// ### Config ID
// `clusterconfig/image`
//
// ### Released version
// - 4.11.0
//
// ### Backported versions
// - 4.10.8+
//
// ### Notes
// None
func (g *Gatherer) GatherClusterImage(ctx context.Context) ([]record.Record, []error) {
	configCli, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherClusterImage(ctx, configCli)
}

func gatherClusterImage(ctx context.Context, configCli configv1client.ConfigV1Interface) ([]record.Record, []error) {
	image, err := configCli.Images().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{
		Name: "config/image",
		Item: record.ResourceMarshaller{Resource: image},
	}}, nil
}
