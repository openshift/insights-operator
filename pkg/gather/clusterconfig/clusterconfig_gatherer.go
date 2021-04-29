package clusterconfig

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/gather/common"
	"github.com/openshift/insights-operator/pkg/record"
)

// Gatherer is an object storing config and having all the gathering functions
type Gatherer struct {
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	anonymizer              *anonymization.Anonymizer
}

// gathererFuncPtr is a type for pointers to functions of Gatherer
type gathererFuncPtr = func(*Gatherer, context.Context) ([]record.Record, []error)

// gatheringFunction describes a gathering function
type gatheringFunction struct {
	CanFail  bool
	Function gathererFuncPtr
}

// importantFunc creates an object describing a gathering function that canNOT fail
func importantFunc(function gathererFuncPtr) gatheringFunction {
	return gatheringFunction{
		CanFail:  false,
		Function: function,
	}
}

// failableFunc creates an object describing a gathering function that can fail
func failableFunc(function gathererFuncPtr) gatheringFunction {
	return gatheringFunction{
		CanFail:  true,
		Function: function,
	}
}

var gatheringFunctions = map[string]gatheringFunction{
	"pdbs":                              importantFunc((*Gatherer).GatherPodDisruptionBudgets),
	"metrics":                           failableFunc((*Gatherer).GatherMostRecentMetrics),
	"operators":                         importantFunc((*Gatherer).GatherClusterOperators),
	"operators_pods_and_events":         importantFunc((*Gatherer).GatherClusterOperatorPodsAndEvents),
	"container_images":                  importantFunc((*Gatherer).GatherContainerImages),
	"nodes":                             importantFunc((*Gatherer).GatherNodes),
	"config_maps":                       failableFunc((*Gatherer).GatherConfigMaps),
	"version":                           importantFunc((*Gatherer).GatherClusterVersion),
	"infrastructures":                   importantFunc((*Gatherer).GatherClusterInfrastructure),
	"networks":                          importantFunc((*Gatherer).GatherClusterNetwork),
	"authentication":                    importantFunc((*Gatherer).GatherClusterAuthentication),
	"image_registries":                  importantFunc((*Gatherer).GatherClusterImageRegistry),
	"image_pruners":                     importantFunc((*Gatherer).GatherClusterImagePruner),
	"feature_gates":                     importantFunc((*Gatherer).GatherClusterFeatureGates),
	"oauths":                            importantFunc((*Gatherer).GatherClusterOAuth),
	"ingress":                           importantFunc((*Gatherer).GatherClusterIngress),
	"proxies":                           importantFunc((*Gatherer).GatherClusterProxy),
	"certificate_signing_requests":      importantFunc((*Gatherer).GatherCertificateSigningRequests),
	"crds":                              importantFunc((*Gatherer).GatherCRD),
	"host_subnets":                      importantFunc((*Gatherer).GatherHostSubnet),
	"machine_sets":                      importantFunc((*Gatherer).GatherMachineSet),
	"install_plans":                     importantFunc((*Gatherer).GatherInstallPlans),
	"service_accounts":                  importantFunc((*Gatherer).GatherServiceAccounts),
	"machine_config_pools":              importantFunc((*Gatherer).GatherMachineConfigPool),
	"container_runtime_configs":         importantFunc((*Gatherer).GatherContainerRuntimeConfig),
	"netnamespaces":                     importantFunc((*Gatherer).GatherNetNamespace),
	"openshift_apiserver_operator_logs": failableFunc((*Gatherer).GatherOpenShiftAPIServerOperatorLogs),
	"openshift_sdn_logs":                failableFunc((*Gatherer).GatherOpenshiftSDNLogs),
	"openshift_sdn_controller_logs":     failableFunc((*Gatherer).GatherOpenshiftSDNControllerLogs),
	"openshift_authentication_logs":     failableFunc((*Gatherer).GatherOpenshiftAuthenticationLogs),
	"sap_config":                        failableFunc((*Gatherer).GatherSAPConfig),
	"sap_license_management_logs":       failableFunc((*Gatherer).GatherSAPVsystemIptablesLogs),
	"sap_pods":                          failableFunc((*Gatherer).GatherSAPPods),
	"sap_datahubs":                      failableFunc((*Gatherer).GatherSAPDatahubs),
	"olm_operators":                     failableFunc((*Gatherer).GatherOLMOperators),
	"pod_network_connectivity_checks":   failableFunc((*Gatherer).GatherPNCC),
}

func New(
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig *rest.Config,
	anonymizer *anonymization.Anonymizer,
) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
		anonymizer:              anonymizer,
	}
}

func (g *Gatherer) GetName() string {
	return "clusterconfig"
}

func (g *Gatherer) GetGatheringFunctions() map[string]common.GatheringClosure {
	result := make(map[string]common.GatheringClosure)

	for funcName, function := range gatheringFunctions {
		function := function

		result[funcName] = common.GatheringClosure{
			Run: func(ctx context.Context) ([]record.Record, []error) {
				return function.Function(g, ctx)
			},
			CanFail: function.CanFail,
		}
	}

	return result
}
