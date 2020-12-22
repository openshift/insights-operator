package clusterconfig

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

type gatherStatusReport struct {
	Name    string        `json:"name"`
	Elapsed time.Duration `json:"elapsed"`
	Report  int           `json:"report"`
	Errors  []error       `json:"errors"`
}

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	ctx                     context.Context
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
}

type gatherFunction func(g *Gatherer) ([]record.Record, []error)

const gatherAll = "ALL"

var gatherFunctions = map[string]gatherFunction{
	"pdbs":                              GatherPodDisruptionBudgets,
	"metrics":                           GatherMostRecentMetrics,
	"operators":                         GatherClusterOperators,
	"container_images":                  GatherContainerImages,
	"nodes":                             GatherNodes,
	"config_maps":                       GatherConfigMaps,
	"version":                           GatherClusterVersion,
	"id":                                GatherClusterID,
	"infrastructures":                   GatherClusterInfrastructure,
	"networks":                          GatherClusterNetwork,
	"authentication":                    GatherClusterAuthentication,
	"image_registries":                  GatherClusterImageRegistry,
	"image_pruners":                     GatherClusterImagePruner,
	"feature_gates":                     GatherClusterFeatureGates,
	"oauths":                            GatherClusterOAuth,
	"ingress":                           GatherClusterIngress,
	"proxies":                           GatherClusterProxy,
	"certificate_signing_requests":      GatherCertificateSigningRequests,
	"crds":                              GatherCRD,
	"host_subnets":                      GatherHostSubnet,
	"machine_sets":                      GatherMachineSet,
	"install_plans":                     GatherInstallPlans,
	"service_accounts":                  GatherServiceAccounts,
	"machine_config_pools":              GatherMachineConfigPool,
	"container_runtime_configs":         GatherContainerRuntimeConfig,
	"stateful_sets":                     GatherStatefulSets,
	"netnamespaces":                     GatherNetNamespace,
	"openshift_apiserver_operator_logs": GatherOpenShiftAPIServerOperatorLogs,
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
func (g *Gatherer) Gather(ctx context.Context, gatherList []string, recorder record.Interface) error {
	g.ctx = ctx
	var errors []string
	var gatherReport []interface{}

	if len(gatherList) == 0 {
		errors = append(errors, "no gather functions are specified to run")
	}

	if contains(gatherList, gatherAll) {
		gatherList = make([]string, 0, len(gatherFunctions))
		for k := range gatherFunctions {
			gatherList = append(gatherList, k)
		}
	}

	for _, gatherId := range gatherList {
		gFn, ok := gatherFunctions[gatherId]
		if !ok {
			errors = append(errors, fmt.Sprintf("unknown gatherId in config: %s", gatherId))
			continue
		}
		gatherName := runtime.FuncForPC(reflect.ValueOf(gFn).Pointer()).Name()
		klog.V(5).Infof("Gathering %s", gatherName)

		start := time.Now()
		records, errs := gFn(g)
		elapsed := time.Since(start).Truncate(time.Millisecond)

		klog.V(4).Infof("Gather %s took %s to process %d records", gatherName, elapsed, len(records))
		gatherReport = append(gatherReport, gatherStatusReport{gatherName, elapsed, len(records), errs})

		for _, err := range errs {
			errors = append(errors, err.Error())
		}
		for _, record := range records {
			if err := recorder.Record(record); err != nil {
				errors = append(errors, fmt.Sprintf("unable to record %s: %v", record.Name, err))
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	// Creates the gathering performance report
	if err := recordGatherReport(recorder, gatherReport); err != nil {
		errors = append(errors, fmt.Sprintf("unable to record io status reports: %v", err))
	}

	if len(errors) > 0 {
		sort.Strings(errors)
		errors = uniqueStrings(errors)
		return fmt.Errorf("%s", strings.Join(errors, ", "))
	}
	return nil
}

func recordGatherReport(recorder record.Interface, report []interface{}) error {
	r := record.Record{Name: "insights-operator/gathers", Item: record.JSONMarshaller{Object: report}}
	return recorder.Record(r)
}

func uniqueStrings(list []string) []string {
	if len(list) < 2 {
		return list
	}
	keys := make(map[string]bool)
	set := []string{}
	for _, entry := range list {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			set = append(set, entry)
		}
	}
	return set
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
