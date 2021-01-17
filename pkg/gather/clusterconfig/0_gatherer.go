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

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/utils"
)

// gatherMetadata contains general information about collected data
type gatherMetadata struct {
	// info about gathering functions
	StatusReports []gathererStatusReport `json:"status_reports"`
	MemoryAlloc   uint64                 `json:"memory_alloc_bytes"`
	Uptime        float64                `json:"uptime_seconds"`
	// shows if obfuscation(hiding IPs and cluster domain) is enabled
	IsGlobalObfuscationEnabled bool `json:"is_global_obfuscation_enabled"`
}

// gathererStatusReport contains general information about specific gatherer function
type gathererStatusReport struct {
	Name         string        `json:"name"`
	Duration     time.Duration `json:"duration_in_ms"`
	RecordsCount int           `json:"records_count"`
	Errors       []string      `json:"errors"`
}

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	ctx                     context.Context
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
	anonymizer              *anonymization.Anonymizer
	startTime               time.Time
}

type gatherResult struct {
	records []record.Record
	errors  []error
}

type gatherFunction func(g *Gatherer, c chan<- gatherResult)
type gathering struct {
	function gatherFunction
	canFail  bool
}

func important(function gatherFunction) gathering {
	return gathering{function, false}
}

func failable(function gatherFunction) gathering {
	return gathering{function, true}
}

const gatherAll = "ALL"

var gatherFunctions = map[string]gathering{
	"pdbs":                              important(GatherPodDisruptionBudgets),
	"metrics":                           failable(GatherMostRecentMetrics),
	"operators":                         important(GatherClusterOperators),
	"container_images":                  important(GatherContainerImages),
	"workload_info":                     important(GatherWorkloadInfo),
	"nodes":                             important(GatherNodes),
	"config_maps":                       failable(GatherConfigMaps),
	"version":                           important(GatherClusterVersion),
	"infrastructures":                   important(GatherClusterInfrastructure),
	"networks":                          important(GatherClusterNetwork),
	"authentication":                    important(GatherClusterAuthentication),
	"image_registries":                  important(GatherClusterImageRegistry),
	"image_pruners":                     important(GatherClusterImagePruner),
	"feature_gates":                     important(GatherClusterFeatureGates),
	"oauths":                            important(GatherClusterOAuth),
	"ingress":                           important(GatherClusterIngress),
	"proxies":                           important(GatherClusterProxy),
	"certificate_signing_requests":      important(GatherCertificateSigningRequests),
	"crds":                              important(GatherCRD),
	"host_subnets":                      important(GatherHostSubnet),
	"machine_sets":                      important(GatherMachineSet),
	"install_plans":                     important(GatherInstallPlans),
	"service_accounts":                  important(GatherServiceAccounts),
	"machine_config_pools":              important(GatherMachineConfigPool),
	"container_runtime_configs":         important(GatherContainerRuntimeConfig),
	"netnamespaces":                     important(GatherNetNamespace),
	"openshift_apiserver_operator_logs": failable(GatherOpenShiftAPIServerOperatorLogs),
	"openshift_sdn_logs":                failable(GatherOpenshiftSDNLogs),
	"openshift_sdn_controller_logs":     failable(GatherOpenshiftSDNControllerLogs),
	"openshift_authentication_logs":     failable(GatherOpenshiftAuthenticationLogs),
	"sap_config":                        failable(GatherSAPConfig),
	"sap_license_management_logs":       failable(GatherSAPVsystemIptablesLogs),
	"sap_pods":                          failable(GatherSAPPods),
	"sap_datahubs":                      failable(GatherSAPDatahubs),
	"olm_operators":                     failable(GatherOLMOperators),
}

// New creates new Gatherer
func New(
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig *rest.Config, anonymizer *anonymization.Anonymizer,
) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
		anonymizer:              anonymizer,
		startTime:               time.Now(),
	}
}

// GatherInfo from reflection
type GatherInfo struct {
	name     string
	result   gatherResult
	function gatherFunction
	canFail  bool
	rvString string
}

// NewGatherInfo that holds reflection information
func NewGatherInfo(gather string, rv reflect.Value) *GatherInfo {
	gatherFunc := gatherFunctions[gather].function
	return &GatherInfo{
		name:     runtime.FuncForPC(reflect.ValueOf(gatherFunc).Pointer()).Name(),
		result:   rv.Interface().(gatherResult),
		function: gatherFunc,
		canFail:  gatherFunctions[gather].canFail,
		rvString: rv.String(),
	}
}

// Gather is hosting and calling all the recording functions
func (g *Gatherer) Gather(ctx context.Context, gatherList []string, recorder recorder.Interface) error {
	g.ctx = ctx
	var errors []string
	var gatherReport gatherMetadata

	if len(gatherList) == 0 {
		errors = append(errors, "no gather functions are specified to run")
	}

	if utils.StringInSlice(gatherAll, gatherList) {
		gatherList = fullGatherList()
	}

	// Starts the gathers in Go routines
	cases, starts, err := g.startGathering(gatherList, &errors)
	if err != nil {
		return err
	}

	// Gets the info from the Go routines
	for range gatherList {
		chosen, value, _ := reflect.Select(cases)
		// The chosen channel has been closed, so zero out the channel to disable the case
		cases[chosen].Chan = reflect.ValueOf(nil)
		gather := gatherList[chosen]

		gi := NewGatherInfo(gather, value)
		statusReport, errorsReport := createStatusReport(gi, recorder, starts[chosen])

		if len(errorsReport) > 0 {
			errors = append(errors, errorsReport...)
		}
		gatherReport.StatusReports = append(gatherReport.StatusReports, statusReport)
	}

	// if obfuscation is enabled, we want to know it from the archive
	gatherReport.IsGlobalObfuscationEnabled = g.anonymizer != nil

	// fill in performance related data to the report
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	gatherReport.MemoryAlloc = m.HeapAlloc
	gatherReport.Uptime = time.Since(g.startTime).Truncate(time.Millisecond).Seconds()

	// records the report
	if err := recordGatherReport(recorder, gatherReport); err != nil {
		errors = append(errors, fmt.Sprintf("unable to record io status reports: %v", err))
	}

	if len(errors) > 0 {
		return sumErrors(errors)
	}

	return nil
}

func createStatusReport(gather *GatherInfo, recorder recorder.Interface, starts time.Time) (gathererStatusReport, []string) {
	var errors []string
	elapsed := time.Since(starts).Truncate(time.Millisecond)

	klog.V(4).Infof("Gather %s took %s to process %d records", gather.name, elapsed, len(gather.result.records))

	shortName := strings.Replace(gather.name, "github.com/openshift/insights-operator/pkg/gather/", "", 1)
	report := gathererStatusReport{shortName, time.Duration(elapsed.Milliseconds()), len(gather.result.records), extractErrors(gather.result.errors)}

	if gather.canFail {
		for _, err := range gather.result.errors {
			klog.V(5).Infof("Couldn't gather %s' received following error: %s\n", gather.name, err.Error())
		}
	} else {
		errors = extractErrors(gather.result.errors)
	}

	errors = append(errors, recordStatusReport(recorder, gather.result.records)...)
	klog.V(5).Infof("Read from %s's channel and received %s\n", gather.name, gather.rvString)

	return report, errors
}

func recordStatusReport(recorder recorder.Interface, records []record.Record) []string {
	var errors []string
	for _, r := range records {
		if err := recorder.Record(r); err != nil {
			errors = append(errors, fmt.Sprintf("unable to record %s: %v", r.Name, err))
			continue
		}
	}
	return errors
}

// Runs each gather functions in a goroutine.
// Every gather function is given its own channel to send back the results.
// 1. return value: `cases` list, used for dynamically reading from the channels.
// 2. return value: `starts` list, contains that start time of each gather function.
func (g *Gatherer) startGathering(gatherList []string, errors *[]string) ([]reflect.SelectCase, []time.Time, error) {
	var cases []reflect.SelectCase
	var starts []time.Time

	// Starts the gathers in Go routines
	for _, gatherID := range gatherList {
		gather, ok := gatherFunctions[gatherID]
		gFn := gather.function
		if !ok {
			*errors = append(*errors, fmt.Sprintf("unknown gatherId in config: %s", gatherID))
			continue
		}
		channel := make(chan gatherResult)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(channel)})
		gatherName := runtime.FuncForPC(reflect.ValueOf(gFn).Pointer()).Name()

		klog.V(5).Infof("Gathering %s", gatherName)
		starts = append(starts, time.Now())
		go gFn(g, channel)

		if err := g.ctx.Err(); err != nil {
			return nil, nil, err
		}
	}

	return cases, starts, nil
}

func recordGatherReport(recorder recorder.Interface, metadata gatherMetadata) error {
	r := record.Record{Name: "insights-operator/gathers", Item: record.JSONMarshaller{Object: metadata}}
	return recorder.Record(r)
}

func extractErrors(errors []error) []string {
	var errStrings []string
	for _, err := range errors {
		errStrings = append(errStrings, err.Error())
	}
	return errStrings
}

func sumErrors(errors []string) error {
	sort.Strings(errors)
	errors = utils.UniqueStrings(errors)
	return fmt.Errorf("%s", strings.Join(errors, ", "))
}

func fullGatherList() []string {
	gatherList := make([]string, 0, len(gatherFunctions))
	for k := range gatherFunctions {
		gatherList = append(gatherList, k)
	}
	return gatherList
}
