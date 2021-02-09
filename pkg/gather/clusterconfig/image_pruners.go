package clusterconfig

import (
	"fmt"
	"strings"
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	registryv1 "github.com/openshift/api/imageregistry/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterImagePruner fetches the image pruner configuration
//
// Location in archive: config/clusteroperator/imageregistry.operator.openshift.io/imagepruner/cluster.json
func GatherClusterImagePruner(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		pruner, err := i.registryClient.ImagePruners().Get(i.ctx, "cluster", metav1.GetOptions{})
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
			Item: ImagePrunerAnonymizer{pruner},
		}}, nil
	}
}

// ImagePrunerAnonymizer implements serialization with marshalling
type ImagePrunerAnonymizer struct {
	*registryv1.ImagePruner
}

// Marshal serializes ImagePruner with anonymization
func (a ImagePrunerAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(registrySerializer.LegacyCodec(registryv1.SchemeGroupVersion), a.ImagePruner)
}

// GetExtension returns extension for anonymized image pruner objects
func (a ImagePrunerAnonymizer) GetExtension() string {
	return "json"
}
