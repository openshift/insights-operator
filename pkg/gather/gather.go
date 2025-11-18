// Package gather contains common gathering logic for all gatherers
package gather

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/api/insights/v1alpha2"
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

// ArchiveMetadata contains the information about the archive and all its gatherers
type ArchiveMetadata struct {
	// info about gathering functions.
	StatusReports []GathererFunctionReport `json:"status_reports"`
	// MemoryBytesUsage is the number of bytes of memory used by the container. The number is obtained
	// from cgroups and is related to the Prometheus metric with the same name.
	MemoryBytesUsage uint64 `json:"container_memory_bytes_usage"`
	// Uptime is the number of seconds from the program start till the point when metadata was created
	Uptime float64 `json:"uptime_seconds"`
	// IsGlobalObfuscationEnabled shows if obfuscation(hiding IPs and cluster domain) is enabled
	IsGlobalObfuscationEnabled bool `json:"is_global_obfuscation_enabled"`
}

// CreateAllGatherers creates all the gatherers
func CreateAllGatherers(
	gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig *rest.Config,
	anonymizer *anonymization.Anonymizer, configObserver configobserver.Interface,
	insightsClient *insightsclient.Client,
) []gatherers.Interface {
	clusterConfigGatherer := clusterconfig.New(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig,
		anonymizer, configObserver,
	)
	workloadsGatherer := workloads.New(gatherKubeConfig, gatherProtoKubeConfig)
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
	gatherConfigs []v1alpha2.GathererConfig,
) ([]GathererFunctionReport, error) {
	startTime := time.Now()
	reports, totalNumberOfRecords, errs := collectAndRecordGatherer(ctx, gatherer, rec, gatherConfigs)
	reports = append(reports, GathererFunctionReport{
		FuncName:     gatherer.GetName(),
		Duration:     time.Since(startTime).Milliseconds(),
		RecordsCount: totalNumberOfRecords,
		Errors:       utils.ErrorsToStrings(errs),
	})

	return reports, utils.UniqueErrors(errs)
}

func collectAndRecordGatherer(
	ctx context.Context,
	gatherer gatherers.Interface,
	rec recorder.Interface,
	gatherConfigs []v1alpha2.GathererConfig,
) (reports []GathererFunctionReport, totalNumberOfRecords int, allErrors []error) {
	resultsChan, err := startGatheringConcurrently(ctx, gatherer, gatherConfigs)
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
	archiveMetadata := record.Record{
		Name:         recorder.MetadataRecordName,
		AlwaysStored: true,
		Item: record.JSONMarshaller{Object: ArchiveMetadata{
			StatusReports:              functionReports,
			Uptime:                     time.Since(programStartTime).Truncate(time.Millisecond).Seconds(),
			IsGlobalObfuscationEnabled: anonymizer.IsAnonymizerTypeEnabled(anonymization.NetworkAnonymizerType),
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
	ctx context.Context, gatherer gatherers.Interface, gatherConfigs []v1alpha2.GathererConfig,
) (chan GatheringFunctionResult, error) {
	var tasks []Task
	var gatheringFunctions map[string]gatherers.GatheringClosure
	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	if err != nil {
		return nil, err
	}

	// This is from TechPreview feature, so we have to check the nil
	if len(gatherConfigs) > 0 {
		gatheringFunctions = getEnabledGatheringFunctions(gatherer.GetName(), gatheringFunctions, gatherConfigs)
	}

	if len(gatheringFunctions) == 0 {
		return nil, fmt.Errorf("no gather functions are specified to run")
	}

	for functionName, gatheringClosure := range gatheringFunctions {
		tasks = append(tasks, Task{
			Name: functionName,
			F:    gatheringClosure,
		})
	}

	return HandleTasksConcurrently(ctx, tasks), nil
}

// getEnabledGatheringFunctions iterates over all gathering functions and
// creates a new map without all the disabled functions
func getEnabledGatheringFunctions(
	gathererName string,
	allGatheringFunctions map[string]gatherers.GatheringClosure,
	gathererConfigs []v1alpha2.GathererConfig,
) map[string]gatherers.GatheringClosure {
	enabledGatheringFunctions := make(map[string]gatherers.GatheringClosure)

	// If the whole gatherer is disabled, check if any function is explicitly enabled
	if hasGathererState(gathererConfigs, gathererName, v1alpha2.GathererStateDisabled) {
		for fName, gatheringClosure := range allGatheringFunctions {
			if hasGathererState(gathererConfigs, fmt.Sprintf("%s/%s", gathererName, fName), v1alpha2.GathererStateEnabled) {
				enabledGatheringFunctions[fName] = gatheringClosure
			}
		}
		return enabledGatheringFunctions
	}

	// Otherwise, enable all functions except those explicitly disabled
	for fName, gatheringClosure := range allGatheringFunctions {
		if !hasGathererState(gathererConfigs, fmt.Sprintf("%s/%s", gathererName, fName), v1alpha2.GathererStateDisabled) {
			enabledGatheringFunctions[fName] = gatheringClosure
		}
	}

	return enabledGatheringFunctions
}

// Checks if the given gathererName has the specified state in gathererConfigs
func hasGathererState(gathererConfigs []v1alpha2.GathererConfig, gathererName string, state v1alpha2.GathererState) bool {
	for _, gf := range gathererConfigs {
		if gf.Name == gathererName && gf.State == state {
			return true
		}
	}
	return false
}

// FunctionReportsMapToArray converts provided map[string]GathererFunctionReport to a slice of
// GathererFunctionReports. Map keys are not used.
func FunctionReportsMapToArray(m map[string]GathererFunctionReport) []GathererFunctionReport {
	a := make([]GathererFunctionReport, 0, len(m))
	for _, v := range m {
		a = append(a, v)
	}
	return a
}
