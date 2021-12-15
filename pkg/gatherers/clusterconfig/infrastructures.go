package clusterconfig

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
func (g *Gatherer) GatherClusterInfrastructure(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterInfrastructure(ctx, gatherConfigClient)
}

func gatherClusterInfrastructure(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]record.Record, []error) {
	config, err := configClient.Infrastructures().Get(ctx, "cluster", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	return []record.Record{{Name: "config/infrastructure", Item: record.ResourceMarshaller{Resource: anonymizeInfrastructure(config)}}}, nil
}

func anonymizeInfrastructure(config *configv1.Infrastructure) *configv1.Infrastructure {
	config.Status.InfrastructureName = anonymize.URL(config.Status.InfrastructureName)

	if config.Status.PlatformStatus.AWS != nil {
		config.Status.PlatformStatus.AWS.Region = anonymize.String(config.Status.PlatformStatus.AWS.Region)
	}
	if config.Status.PlatformStatus.Azure != nil {
		config.Status.PlatformStatus.Azure.CloudName = configv1.AzureCloudEnvironment(
			anonymize.String(string(config.Status.PlatformStatus.Azure.CloudName)),
		)
	}
	if config.Status.PlatformStatus.GCP != nil {
		config.Status.PlatformStatus.GCP.Region = anonymize.String(config.Status.PlatformStatus.GCP.Region)
		config.Status.PlatformStatus.GCP.ProjectID = anonymize.String(config.Status.PlatformStatus.GCP.ProjectID)
	}
	if config.Status.PlatformStatus.IBMCloud != nil {
		config.Status.PlatformStatus.IBMCloud.Location = anonymize.String(config.Status.PlatformStatus.IBMCloud.Location)
	}
	if config.Status.PlatformStatus.OpenStack != nil {
		config.Status.PlatformStatus.OpenStack.CloudName = anonymize.String(config.Status.PlatformStatus.OpenStack.CloudName)
	}

	return config
}
