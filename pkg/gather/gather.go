// gather package contains common gathering logic for all gatherers
package gather

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/clusterconfig"
	"github.com/openshift/insights-operator/pkg/gatherers/workloads"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/utils"
)

// norevive
const (
	AllGatherersConst = "ALL"
)

var programStartTime = time.Now()

// GathererFunctionReport contains the information about a specific gathering function
type GathererFunctionReport struct {
	FuncName     string      `json:"name"`
	Duration     int64       `json:"duration_in_ms"`
	RecordsCount int         `json:"records_count"`
	Errors       []string    `json:"errors"`
	Panic        interface{} `json:"panic"`
}

// ArchiveMetadata contains the information about the archive and all its' gatherers
type ArchiveMetadata struct {
	// info about gathering functions.
	StatusReports []GathererFunctionReport `json:"status_reports"`
	// MemoryAlloc is the amount of memory taken by heap objects after processing the records
	MemoryAlloc uint64 `json:"memory_alloc_bytes"`
	// Uptime is the number of seconds from the program start till the point when metadata was created
	Uptime float64 `json:"uptime_seconds"`
	// IsGlobalObfuscationEnabled shows if obfuscation(hiding IPs and cluster domain) is enabled
	IsGlobalObfuscationEnabled bool `json:"is_global_obfuscation_enabled"`
}

// CreateAllGatherers creates all the gatherers
func CreateAllGatherers(
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig *rest.Config,
	anonymizer *anonymization.Anonymizer,
) []gatherers.Interface {
	clusterConfigGatherer := clusterconfig.New(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, anonymizer,
	)
	workloadsGatherer := workloads.New(gatherProtoKubeConfig)

	return []gatherers.Interface{clusterConfigGatherer, workloadsGatherer}
}

// CollectAndRecordGatherer gathers enabled functions of the provided gatherer and records the results to the recorder
// and returns info about the recorded data. Panics are just logged and written
// to the resulting array (to the archive metadata)
func CollectAndRecordGatherer(
	ctx context.Context,
	gatherer gatherers.Interface,
	rec recorder.Interface,
	configurator configobserver.Configurator,
) ([]GathererFunctionReport, error) {
	resultsChan, err := startGatheringConcurrently(ctx, gatherer, configurator.Config().Gather)
	if err != nil {
		return nil, err
	}

	gathererName := gatherer.GetName()

	var errs []error
	var functionReports []GathererFunctionReport

	for result := range resultsChan {
		if result.Panic != nil {
			klog.Error(fmt.Errorf(
				"gatherer %v's function %v panicked with error: %v",
				gathererName, result.FunctionName, result.Panic,
			))
			result.Errs = append(result.Errs, fmt.Errorf("%v", result.Panic))
		}

		for _, err := range result.Errs {
			errStr := fmt.Sprintf(
				"gatherer %v's function %v failed with error: %v",
				gathererName, result.FunctionName, err,
			)

			if result.IgnoreErrors {
				klog.Error(errStr)
			} else {
				errs = append(errs, fmt.Errorf(errStr))
			}
		}
		recordedRecs := 0
		for _, r := range result.Records {
			if err := rec.Record(r); err != nil {
				recErr := fmt.Errorf(
					"unable to record gatherer %v function %v' result %v because of error: %v",
					gathererName, result.FunctionName, r.Name, err,
				)
				result.Errs = append(result.Errs, recErr)
				continue
			}
			recordedRecs++
		}

		klog.Infof(
			"Gather %v's function %v took %v to process %v records",
			gathererName, result.FunctionName, result.TimeElapsed, len(result.Records),
		)

		functionReports = append(functionReports, GathererFunctionReport{
			FuncName:     fmt.Sprintf("%v/%v", gathererName, result.FunctionName),
			Duration:     result.TimeElapsed.Milliseconds(),
			RecordsCount: recordedRecs,
			Errors:       errorsToStrings(result.Errs),
			Panic:        result.Panic,
		})
	}
	return functionReports, sumErrors(errs)
}

// RecordArchiveMetadata records info about archive and gatherers' reports
func RecordArchiveMetadata(
	functionReports []GathererFunctionReport,
	rec recorder.Interface,
	anonymizer *anonymization.Anonymizer,
) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	archiveMetadata := record.Record{
		Name: recorder.MetadataRecordName,
		Item: record.JSONMarshaller{Object: ArchiveMetadata{
			StatusReports:              functionReports,
			MemoryAlloc:                m.HeapAlloc,
			Uptime:                     time.Since(programStartTime).Truncate(time.Millisecond).Seconds(),
			IsGlobalObfuscationEnabled: anonymizer != nil,
		}},
	}
	if err := rec.Record(archiveMetadata); err != nil {
		return fmt.Errorf("unable to record archive metadata because of error: %v", err)
	}

	return nil
}

// startGatheringConcurrently starts gathering of enabled functions of the provided gatherer and returns a channel
// with results which will be closed when processing is done
func startGatheringConcurrently(
	ctx context.Context, gatherer gatherers.Interface, enabledFunctions []string,
) (chan GatheringFunctionResult, error) {
	gathererName := gatherer.GetName()

	gatherAllFunctions, gatherFunctionsList := getListOfEnabledFunctionForGatherer(
		gathererName, enabledFunctions,
	)
	if !gatherAllFunctions && len(gatherFunctionsList) == 0 {
		return nil, fmt.Errorf("no gather functions are specified to run")
	}

	var tasks []Task

	for functionName, gatheringClosure := range gatherer.GetGatheringFunctions() {
		if !gatherAllFunctions && !utils.StringInSlice(functionName, gatherFunctionsList) {
			continue
		}

		tasks = append(tasks, Task{
			Name: functionName,
			F:    gatheringClosure,
		})
	}

	return HandleTasksConcurrently(ctx, tasks), nil
}

// getListOfEnabledFunctionForGatherer parses a list of gathering functions to enable,
// which has the following structure: []string {
//   "clusterconfig/container_images",
//   "clusterconfig/nodes",
//   "clusterconfig/authentication",
//   "othergatherer/some_function",
// } where each item consists of a gatherer name and a function name split by a slash.
// If there's a string "ALL", we enable everything and return the first parameter as true,
// otherwise it will be false and the second parameter will contain function names
func getListOfEnabledFunctionForGatherer(gathererName string, allFunctionsList []string) (ok bool, list []string) {
	if utils.StringInSlice(AllGatherersConst, allFunctionsList) {
		return true, nil
	}

	var result []string

	for _, functionName := range allFunctionsList {
		prefix := gathererName + "/"
		if strings.HasPrefix(functionName, prefix) {
			result = append(result, strings.TrimPrefix(functionName, prefix))
		}
	}

	return false, result
}

// sumErrors simply sorts the errors and joins them with commas
func sumErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	var errStrings []string
	for _, err := range errs {
		errStrings = append(errStrings, err.Error())
	}

	sort.Strings(errStrings)
	errStrings = utils.UniqueStrings(errStrings)

	return fmt.Errorf("%s", strings.Join(errStrings, ", "))
}

// errorsToStrings turns error slice to string slice
func errorsToStrings(errs []error) []string {
	var result []string
	for _, err := range errs {
		result = append(result, err.Error())
	}

	return result
}
