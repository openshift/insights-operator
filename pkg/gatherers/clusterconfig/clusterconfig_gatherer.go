package clusterconfig

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
)

// Gatherer is an object storing config and having all the gathering functions
type Gatherer struct {
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	alertsGatherKubeConfig  *rest.Config
	anonymizer              *anonymization.Anonymizer
	configAggregator        configobserver.Interface
}

// gathererFuncPtr is a type for pointers to functions of Gatherer
type gathererFuncPtr = func(*Gatherer, context.Context) ([]record.Record, []error)

var gatheringFunctions = map[string]gathererFuncPtr{
	"active_alerts":                    (*Gatherer).GatherActiveAlerts,
	"aggregated_monitoring_cr_names":   (*Gatherer).GatherAggregatedMonitoringCRNames,
	"authentication":                   (*Gatherer).GatherClusterAuthentication,
	"certificate_signing_requests":     (*Gatherer).GatherCertificateSigningRequests,
	"ceph_cluster":                     (*Gatherer).GatherCephCluster,
	"cluster_apiserver":                (*Gatherer).GatherClusterAPIServer,
	"clusterroles":                     (*Gatherer).GatherClusterRoles,
	"config_maps":                      (*Gatherer).GatherConfigMaps,
	"container_images":                 (*Gatherer).GatherContainerImages,
	"container_runtime_configs":        (*Gatherer).GatherContainerRuntimeConfig,
	"cost_management_metrics_configs":  (*Gatherer).GatherCostManagementMetricsConfigs,
	"crds":                             (*Gatherer).GatherCRD,
	"dvo_metrics":                      (*Gatherer).GatherDVOMetrics,
	"feature_gates":                    (*Gatherer).GatherClusterFeatureGates,
	"image":                            (*Gatherer).GatherClusterImage,
	"image_pruners":                    (*Gatherer).GatherClusterImagePruner,
	"image_registries":                 (*Gatherer).GatherClusterImageRegistry,
	"infrastructures":                  (*Gatherer).GatherClusterInfrastructure,
	"ingress":                          (*Gatherer).GatherClusterIngress,
	"ingress_certificates":             (*Gatherer).GatherClusterIngressCertificates,
	"install_plans":                    (*Gatherer).GatherInstallPlans,
	"jaegers":                          (*Gatherer).GatherJaegerCR,
	"lokistack":                        (*Gatherer).GatherLokiStack,
	"machine_autoscalers":              (*Gatherer).GatherMachineAutoscalers,
	"machine_config_pools":             (*Gatherer).GatherMachineConfigPool,
	"machine_configs":                  (*Gatherer).GatherMachineConfigs,
	"machine_healthchecks":             (*Gatherer).GatherMachineHealthCheck,
	"machine_sets":                     (*Gatherer).GatherMachineSet,
	"machines":                         (*Gatherer).GatherMachine,
	"metrics":                          (*Gatherer).GatherMostRecentMetrics,
	"monitoring_persistent_volumes":    (*Gatherer).GatherMonitoringPVs,
	"mutating_webhook_configurations":  (*Gatherer).GatherMutatingWebhookConfigurations,
	"networks":                         (*Gatherer).GatherClusterNetwork,
	"node_logs":                        (*Gatherer).GatherNodeLogs,
	"nodes":                            (*Gatherer).GatherNodes,
	"nodenetworkconfigurationpolicies": (*Gatherer).GatherNodeNetworkConfigurationPolicy,
	"nodenetworkstates":                (*Gatherer).GatherNodeNetworkState,
	"number_of_pods_and_netnamespaces_with_sdn_annotations": (*Gatherer).GatherNumberOfPodsAndNetnamespacesWithSDNAnnotations,
	"oauths":                            (*Gatherer).GatherClusterOAuth,
	"olm_operators":                     (*Gatherer).GatherOLMOperators,
	"openshift_logging":                 (*Gatherer).GatherOpenshiftLogging,
	"openshift_machine_api_events":      (*Gatherer).GatherOpenshiftMachineAPIEvents,
	"openstack_controlplanes":           (*Gatherer).GatherOpenstackControlplanes,
	"openstack_dataplanedeployments":    (*Gatherer).GatherOpenstackDataplaneDeployments,
	"openstack_dataplanenodesets":       (*Gatherer).GatherOpenstackDataplaneNodeSets,
	"openstack_version":                 (*Gatherer).GatherOpenstackVersions,
	"operators":                         (*Gatherer).GatherClusterOperators,
	"operators_pods_and_events":         (*Gatherer).GatherClusterOperatorPodsAndEvents,
	"overlapping_namespace_uids":        (*Gatherer).GatherNamespacesWithOverlappingUIDs,
	"pdbs":                              (*Gatherer).GatherPodDisruptionBudgets,
	"pod_network_connectivity_checks":   (*Gatherer).GatherPodNetworkConnectivityChecks,
	"proxies":                           (*Gatherer).GatherClusterProxy,
	"qemu_kubevirt_launcher_logs":       (*Gatherer).GatherQEMUKubeVirtLauncherLogs,
	"sap_config":                        (*Gatherer).GatherSAPConfig,
	"sap_datahubs":                      (*Gatherer).GatherSAPDatahubs,
	"sap_pods":                          (*Gatherer).GatherSAPPods,
	"schedulers":                        (*Gatherer).GatherSchedulers,
	"service_accounts":                  (*Gatherer).GatherServiceAccounts,
	"silenced_alerts":                   (*Gatherer).GatherSilencedAlerts,
	"storage_classes":                   (*Gatherer).GatherStorageClasses,
	"storage_cluster":                   (*Gatherer).GatherStorageCluster,
	"support_secret":                    (*Gatherer).GatherSupportSecret,
	"tsdb_status":                       (*Gatherer).GatherPrometheusTSDBStatus,
	"validating_webhook_configurations": (*Gatherer).GatherValidatingWebhookConfigurations,
	"version":                           (*Gatherer).GatherClusterVersion,
	"nodefeatures":                      (*Gatherer).GatherNodeFeatures,
}

func New(
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig *rest.Config,
	anonymizer *anonymization.Anonymizer, configObserver configobserver.Interface,
) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
		alertsGatherKubeConfig:  alertsGatherKubeConfig,
		anonymizer:              anonymizer,
		configAggregator:        configObserver,
	}
}

func (g *Gatherer) GetName() string {
	return "clusterconfig"
}

func (g *Gatherer) GetGatheringFunctions(context.Context) (map[string]gatherers.GatheringClosure, error) {
	result := make(map[string]gatherers.GatheringClosure)

	for funcName, function := range gatheringFunctions {
		result[funcName] = gatherers.GatheringClosure{
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return function(g, ctx)
			},
		}
	}

	return result, nil
}

func (g *Gatherer) config() *config.InsightsConfiguration {
	return g.configAggregator.Config()
}
