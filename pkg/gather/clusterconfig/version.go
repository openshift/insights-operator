package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// GatherClusterVersion fetches the ClusterVersion (including the cluster ID) with the name 'version' and its resources.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/clusterversion.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#clusterversion-v1config-openshift-io
//
// * Location in archive: config/version/
// * See: docs/insights-archive-sample/config/version
// * Location of cluster ID: config/id
// * See: docs/insights-archive-sample/config/id
// * Id in config: version
func GatherClusterVersion(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := getClusterVersion(g.ctx, gatherConfigClient)
	c <- gatherResult{records, errors}
}

func getClusterVersion(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/version", Item: record.JSONMarshaller{Object: anonymizeClusterVersion(config)}},
	}

	if config.Spec.ClusterID != "" {
		records = append(records, record.Record{Name: "config/id", Item: marshal.Raw{Str: string(config.Spec.ClusterID)}})
	}

	return records, nil
}

func anonymizeClusterVersion(version *configv1.ClusterVersion) *configv1.ClusterVersion {
	version.Spec.Upstream = configv1.URL(anonymize.AnonymizeURL(string(version.Spec.Upstream)))
	return version
}
