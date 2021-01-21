package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterVersion fetches the ClusterVersion - the ClusterVersion with name version.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusterversion.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusterversion-v1config-openshift-io
//
// Location in archive: config/version/
// See: docs/insights-archive-sample/config/version
// Id in config: version
func GatherClusterVersion(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	config, err := GetClusterVersion(g.ctx, g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	c <- gatherResult{[]record.Record{{Name: "config/version", Item: ClusterVersionAnonymizer{config}}}, nil}
}

func GetClusterVersion(ctx context.Context, kubeConfig *rest.Config) (*configv1.ClusterVersion, error) {
	gatherConfigClient, err := configv1client.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	config, err := gatherConfigClient.ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}

// GatherClusterID stores ClusterID from ClusterVersion version
// This method uses data already collected by Get ClusterVersion. In particular field .Spec.ClusterID
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusterversion.go#L50
// Response see https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go#L38
//
// Location in archive: config/id/
// See: docs/insights-archive-sample/config/id
// Id in config: id
func GatherClusterID(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	version, err := GetClusterVersion(g.ctx, g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	if version == nil {
		c <- gatherResult{nil, nil}
		return
	}
	c <- gatherResult{[]record.Record{{Name: "config/id", Item: Raw{string(version.Spec.ClusterID)}}}, nil}
}

// ClusterVersionAnonymizer is serializing ClusterVersion with anonymization
type ClusterVersionAnonymizer struct{ *configv1.ClusterVersion }

// Marshal serializes ClusterVersion with anonymization
func (a ClusterVersionAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.ClusterVersion.Spec.Upstream = configv1.URL(anonymizeURL(string(a.ClusterVersion.Spec.Upstream)))
	return runtime.Encode(openshiftSerializer, a.ClusterVersion)
}

// GetExtension returns extension for anonymized cluster version objects
func (a ClusterVersionAnonymizer) GetExtension() string {
	return "json"
}
