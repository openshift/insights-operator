package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	imageregistryv1client "github.com/openshift/client-go/imageregistry/clientset/versioned"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterImagePruner Collects the image pruner configuration.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/clusteroperator/imageregistry.operator.openshift.io/imagepruner/cluster.json
//
// ### Location in archive
// | Version   | Path																			     |
// | --------- | ----------------------------------------------------------------------------------- |
// | <= 4.5    | config/imagepruner.json														 	 |
// | >= 4.6    | config/clusteroperator/{group}/{kind}/{name}.json |
//
// ### Config ID
// `clusterconfig/image_pruners`
//
// ### Released version
// - 4.5
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherClusterImagePruner(ctx context.Context) ([]record.Record, []error) {
	registryClient, err := imageregistryv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterImagePruner(ctx, registryClient.ImageregistryV1())
}

func gatherClusterImagePruner(ctx context.Context, registryClient imageregistryv1.ImageregistryV1Interface) ([]record.Record, []error) {
	pruner, err := registryClient.ImagePruners().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	// TypeMeta is empty - see https://github.com/kubernetes/kubernetes/issues/3030
	kinds, _, err := registryScheme.ObjectKinds(pruner)
	if err != nil {
		return nil, []error{err}
	}
	if len(kinds) > 1 {
		klog.Warningf("More kinds for image registry pruner operator resource %s", kinds)
	}
	objKind := kinds[0]
	return []record.Record{{
		Name: fmt.Sprintf("config/clusteroperator/%s/%s/%s", objKind.Group, strings.ToLower(objKind.Kind), pruner.Name),
		Item: record.ResourceMarshaller{Resource: pruner},
	}}, nil
}
