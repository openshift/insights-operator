package clusterconfig

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	imageregistryv1client "github.com/openshift/client-go/imageregistry/clientset/versioned"
	imageregistryv1 "github.com/openshift/client-go/imageregistry/clientset/versioned/typed/imageregistry/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterImagePruner fetches the image pruner configuration
//
// * Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/imagepruner/cluster.json
// * Id in config: image_pruners
func GatherClusterImagePruner(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	registryClient, err := imageregistryv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherClusterImagePruner(g.ctx, registryClient.ImageregistryV1())
	c <- gatherResult{records, errors}
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
		Item: record.JSONMarshaller{Object: pruner},
	}}, nil
}
