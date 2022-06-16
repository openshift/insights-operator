// Package gather contains common gathering logic for all gatherers
package gather

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/clusterconfig"
	"github.com/openshift/insights-operator/pkg/gatherers/conditional"
	"github.com/openshift/insights-operator/pkg/gatherers/workloads"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/types"
	"github.com/openshift/insights-operator/pkg/utils"
)

// norevive
const (
	// AllGatherersConst is used to specify in the config that we want to enable
	// all gathering functions from all gatherers
	AllGatherersConst = "ALL"
)

var programStartTime = time.Now()

// GathererFunctionReport contains the information about a specific gathering function
type GathererFunctionReport struct {
	FuncName     string      `json:"name"`
	Duration     int64       `json:"duration_in_ms"`
	RecordsCount int         `json:"records_count"`
	Errors       []string    `json:"errors"`
	Warnings     []string    `json:"warnings"`
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
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig *rest.Config,
	anonymizer *anonymization.Anonymizer, configObserver *configobserver.Controller,
	insightsClient *insightsclient.Client,
) []gatherers.Interface {
	clusterConfigGatherer := clusterconfig.New(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig,
		anonymizer, configObserver,
	)
	workloadsGatherer := workloads.New(gatherProtoKubeConfig)
	conditionalGatherer := conditional.New(
		gatherProtoKubeConfig, metricsGatherKubeConfig, gatherKubeConfig, configObserver, insightsClient,
	)

	return []gatherers.Interface{clusterConfigGatherer, workloadsGatherer, conditionalGatherer}
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
	startTime := time.Now()
	reports, totalNumberOfRecords, errs := collectAndRecordGatherer(ctx, gatherer, rec, configurator)
	reports = append(reports, GathererFunctionReport{
		FuncName:     gatherer.GetName(),
		Duration:     time.Since(startTime).Milliseconds(),
		RecordsCount: totalNumberOfRecords,
		Errors:       utils.ErrorsToStrings(errs),
	})

	return reports, utils.SumErrors(errs)
}

func collectAndRecordGatherer(
	ctx context.Context,
	gatherer gatherers.Interface,
	rec recorder.Interface,
	configurator configobserver.Configurator,
) (reports []GathererFunctionReport, totalNumberOfRecords int, allErrors []error) {
	resultsChan, err := startGatheringConcurrently(ctx, gatherer, configurator.Config().Gather)
	if err != nil {
		allErrors = append(allErrors, err)
		return reports, totalNumberOfRecords, allErrors
	}

	for result := range resultsChan {
		report, errs := recordGatheringFunctionResult(rec, &result, gatherer.GetName())
		allErrors = append(allErrors, errs...)
		reports = append(reports, report)
		totalNumberOfRecords += report.RecordsCount
	}

	return reports, totalNumberOfRecords, allErrors
}

func recordGatheringFunctionResult(
	rec recorder.Interface, result *GatheringFunctionResult, gathererName string,
) (GathererFunctionReport, []error) {
	var allErrors []error
	var recordWarnings []error
	var recordErrs []error

	if result.Panic != nil {
		recordErrs = append(recordErrs, fmt.Errorf("panic: %v", result.Panic))
		klog.Error(fmt.Errorf(
			`gatherer "%v" function "%v" panicked with the error: %v`,
			gathererName, result.FunctionName, result.Panic,
		))
		allErrors = append(allErrors, fmt.Errorf(`function "%v" panicked`, result.FunctionName))
	}

	for _, err := range result.Errs {
		if w, isWarning := err.(*types.Warning); isWarning {
			recordWarnings = append(recordWarnings, w)
			klog.Warningf(
				`gatherer "%v" function "%v" produced the warning: %v`, gathererName, result.FunctionName, w,
			)
		} else {
			recordErrs = append(recordErrs, err)
			klog.Errorf(
				`gatherer "%v" function "%v" failed with the error: %v`,
				gathererName, result.FunctionName, err,
			)
			allErrors = append(allErrors, fmt.Errorf(`function "%v" failed with an error`, result.FunctionName))
		}
	}

	recordedRecs := 0
	for _, r := range result.Records {
		wasRecorded := true
		if errs := rec.Record(r); len(errs) > 0 {
			for _, err := range errs {
				if w, isWarning := err.(*types.Warning); isWarning {
					recordWarnings = append(recordWarnings, w)
					klog.Warningf(
						`issue recording gatherer "%v" function "%v" result "%v" because of the warning: %v`,
						gathererName, result.FunctionName, r.GetFilename(), w,
					)
				} else {
					recordErrs = append(recordErrs, err)
					klog.Errorf(
						`error recording gatherer "%v" function "%v" result "%v" because of the error: %v`,
						gathererName, result.FunctionName, r.GetFilename(), err,
					)
					allErrors = append(allErrors, fmt.Errorf(
						`unable to record function "%v" record "%v"`, result.FunctionName, r.GetFilename(),
					))
					wasRecorded = false
				}
			}
		}
		if wasRecorded {
			recordedRecs++
		}
	}

	klog.Infof(
		`gatherer "%v" function "%v" took %v to process %v records`,
		gathererName, result.FunctionName, result.TimeElapsed, len(result.Records),
	)

	return GathererFunctionReport{
		FuncName:     fmt.Sprintf("%v/%v", gathererName, result.FunctionName),
		Duration:     result.TimeElapsed.Milliseconds(),
		RecordsCount: recordedRecs,
		Errors:       utils.ErrorsToStrings(recordErrs),
		Warnings:     utils.ErrorsToStrings(recordWarnings),
		Panic:        result.Panic,
	}, allErrors
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
	if errs := rec.Record(archiveMetadata); len(errs) > 0 {
		return fmt.Errorf("unable to record archive metadata because of the errors: %v", errs)
	}

	return nil
}

// startGatheringConcurrently starts gathering of enabled functions of the provided gatherer and returns a channel
// with results which will be closed when processing is done
func startGatheringConcurrently(
	ctx context.Context, gatherer gatherers.Interface, enabledFunctions []string,
) (chan GatheringFunctionResult, error) {
	gathererName := gatherer.GetName()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(
			`unable to start gathering of the gatherer "%v", context has the error: %v`, gathererName, err,
		)
	}

	gatherAllFunctions, gatherFunctionsList := getListOfEnabledFunctionForGatherer(
		gathererName, enabledFunctions,
	)
	if !gatherAllFunctions && len(gatherFunctionsList) == 0 {
		return nil, fmt.Errorf("no gather functions are specified to run")
	}

	var tasks []Task

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	if err != nil {
		return nil, err
	}

	for functionName, gatheringClosure := range gatheringFunctions {
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
