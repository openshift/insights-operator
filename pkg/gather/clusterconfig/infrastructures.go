package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherClusterInfrastructure fetches the cluster Infrastructure - the Infrastructure with name cluster.
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/config/clientset/versioned/typed/config/v1/infrastructure.go#L50
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#infrastructure-v1-config-openshift-io
//
// * Location in archive: config/infrastructure/
// * See: docs/insights-archive-sample/config/infrastructure
// * Id in config: infrastructures
func GatherClusterInfrastructure(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errs := gatherClusterInfrastructure(g.ctx, gatherConfigClient)
	c <- gatherResult{records, errs}
}

func gatherClusterInfrastructure(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.Infrastructures().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{Name: "config/infrastructure", Item: record.JSONMarshaller{Object: anonymizeInfrastructure(config)}}}, nil
}

func anonymizeInfrastructure(config *configv1.Infrastructure) *configv1.Infrastructure {
	config.Status.EtcdDiscoveryDomain = anonymize.AnonymizeURL(config.Status.EtcdDiscoveryDomain)
	config.Status.InfrastructureName = anonymize.AnonymizeURL(config.Status.InfrastructureName)
	return config
}
