package clusterconfig

import (
	"context"
	"time"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/anonymization"
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
	interval                time.Duration
}

// gathererFuncPtr is a type for pointers to functions of Gatherer
type gathererFuncPtr = func(*Gatherer, context.Context) ([]record.Record, []error)

var gatheringFunctions = map[string]gathererFuncPtr{
	"pdbs":                              (*Gatherer).GatherPodDisruptionBudgets,
	"metrics":                           (*Gatherer).GatherMostRecentMetrics,
	"dvo_metrics":                       (*Gatherer).GatherDVOMetrics,
	"operators":                         (*Gatherer).GatherClusterOperators,
	"operators_pods_and_events":         (*Gatherer).GatherClusterOperatorPodsAndEvents,
	"container_images":                  (*Gatherer).GatherContainerImages,
	"nodes":                             (*Gatherer).GatherNodes,
	"config_maps":                       (*Gatherer).GatherConfigMaps,
	"version":                           (*Gatherer).GatherClusterVersion,
	"infrastructures":                   (*Gatherer).GatherClusterInfrastructure,
	"networks":                          (*Gatherer).GatherClusterNetwork,
	"authentication":                    (*Gatherer).GatherClusterAuthentication,
	"image_registries":                  (*Gatherer).GatherClusterImageRegistry,
	"image_pruners":                     (*Gatherer).GatherClusterImagePruner,
	"feature_gates":                     (*Gatherer).GatherClusterFeatureGates,
	"oauths":                            (*Gatherer).GatherClusterOAuth,
	"ingress":                           (*Gatherer).GatherClusterIngress,
	"proxies":                           (*Gatherer).GatherClusterProxy,
	"certificate_signing_requests":      (*Gatherer).GatherCertificateSigningRequests,
	"crds":                              (*Gatherer).GatherCRD,
	"host_subnets":                      (*Gatherer).GatherHostSubnet,
	"machine_sets":                      (*Gatherer).GatherMachineSet,
	"machine_configs":                   (*Gatherer).GatherMachineConfigs,
	"machine_healthchecks":              (*Gatherer).GatherMachineHealthCheck,
	"install_plans":                     (*Gatherer).GatherInstallPlans,
	"service_accounts":                  (*Gatherer).GatherServiceAccounts,
	"machine_config_pools":              (*Gatherer).GatherMachineConfigPool,
	"container_runtime_configs":         (*Gatherer).GatherContainerRuntimeConfig,
	"netnamespaces":                     (*Gatherer).GatherNetNamespace,
	"openshift_apiserver_operator_logs": (*Gatherer).GatherOpenShiftAPIServerOperatorLogs,
	"openshift_sdn_logs":                (*Gatherer).GatherOpenshiftSDNLogs,
	"openshift_sdn_controller_logs":     (*Gatherer).GatherOpenshiftSDNControllerLogs,
	"openshift_authentication_logs":     (*Gatherer).GatherOpenshiftAuthenticationLogs,
	"sap_config":                        (*Gatherer).GatherSAPConfig,
	"sap_license_management_logs":       (*Gatherer).GatherSAPVsystemIptablesLogs,
	"sap_pods":                          (*Gatherer).GatherSAPPods,
	"sap_datahubs":                      (*Gatherer).GatherSAPDatahubs,
	"olm_operators":                     (*Gatherer).GatherOLMOperators,
	"pod_network_connectivity_checks":   (*Gatherer).GatherPNCC,
	"machine_autoscalers":               (*Gatherer).GatherMachineAutoscalers,
	"openshift_logging":                 (*Gatherer).GatherOpenshiftLogging,
	"psps":                              (*Gatherer).GatherPodSecurityPolicies,
	"jaegers":                           (*Gatherer).GatherJaegerCR,
	"validating_webhook_configurations": (*Gatherer).GatherValidatingWebhookConfigurations,
	"mutating_webhook_configurations":   (*Gatherer).GatherMutatingWebhookConfigurations,
	"cost_management_metrics_configs":   (*Gatherer).GatherCostManagementMetricsConfigs,
	"node_logs":                         (*Gatherer).GatherNodeLogs,
	"tsdb_status":                       (*Gatherer).GatherTSDBStatus,
	"schedulers":                        (*Gatherer).GatherSchedulers,
	"scheduler_logs":                    (*Gatherer).GatherSchedulerLogs,
	"silenced_alerts":                   (*Gatherer).GatherSilencedAlerts,
	"image":                             (*Gatherer).GatherClusterImage,
	"kube_controller_manager_logs":      (*Gatherer).GatherKubeControllerManagerLogs,
	"overlapping_namespace_uids":        (*Gatherer).GatherNamespacesWithOverlappingUIDs,
}

func New(
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig *rest.Config,
	anonymizer *anonymization.Anonymizer, interval time.Duration,
) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
		alertsGatherKubeConfig:  alertsGatherKubeConfig,
		anonymizer:              anonymizer,
		interval:                interval,
	}
}

func (g *Gatherer) GetName() string {
	return "clusterconfig"
}

func (g *Gatherer) GetGatheringFunctions(context.Context) (map[string]gatherers.GatheringClosure, error) {
	result := make(map[string]gatherers.GatheringClosure)

	for funcName, function := range gatheringFunctions {
		function := function

		result[funcName] = gatherers.GatheringClosure{
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return function(g, ctx)
			},
		}
	}

	return result, nil
}
