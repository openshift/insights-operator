package clusterconfig

import (
	"context"
	"sync"

	"k8s.io/client-go/rest"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	ctx                     context.Context
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	lock                    sync.Mutex
	lastVersion             *configv1.ClusterVersion
}

// New creates new Gatherer
func New(gatherKubeConfig *rest.Config, gatherProtoKubeConfig *rest.Config, metricsGatherKubeConfig *rest.Config) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
	}
}

// Gather is hosting and calling all the recording functions
func (g *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	g.ctx = ctx
	return record.Collect(ctx, recorder,
		GatherPodDisruptionBudgets(g),
		GatherMostRecentMetrics(g),
		GatherClusterOperators(g),
		GatherContainerImages(g),
		GatherNodes(g),
		GatherConfigMaps(g),
		GatherClusterVersion(g),
		GatherClusterID(g),
		GatherClusterInfrastructure(g),
		GatherClusterNetwork(g),
		GatherClusterAuthentication(g),
		GatherClusterImageRegistry(g),
		GatherClusterImagePruner(g),
		GatherClusterFeatureGates(g),
		GatherClusterOAuth(g),
		GatherClusterIngress(g),
		GatherClusterProxy(g),
		GatherCertificateSigningRequests(g),
		GatherCRD(g),
		GatherHostSubnet(g),
		GatherMachineSet(g),
		GatherMachineConfigPool(g),
		GatherInstallPlans(g),
		GatherContainerRuntimeConfig(g),
		GatherOpenshiftSDNLogs(g),
		GatherNetNamespace(g),
		GatherServiceAccounts(g),
		GatherSAPConfig(g),
		GatherSAPVsystemIptablesLogs(g),
		GatherOpenshiftSDNControllerLogs(g),
	)
}

func (g *Gatherer) setClusterVersion(version *configv1.ClusterVersion) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.lastVersion != nil && g.lastVersion.ResourceVersion == version.ResourceVersion {
		return
	}
	g.lastVersion = version.DeepCopy()
}

// ClusterVersion returns Version for this cluster, which is set by running version during Gathering
func (g *Gatherer) ClusterVersion() *configv1.ClusterVersion {
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.lastVersion
}
