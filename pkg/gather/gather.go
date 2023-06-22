// Package gather contains common gathering logic for all gatherers
package gather

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/api/config/v1alpha1"
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
	gatherConfig *v1alpha1.GatherConfig,
) ([]GathererFunctionReport, error) {
	startTime := time.Now()
	reports, totalNumberOfRecords, errs := collectAndRecordGatherer(ctx, gatherer, rec, gatherConfig)
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
	gatherConfig *v1alpha1.GatherConfig,
) (reports []GathererFunctionReport, totalNumberOfRecords int, allErrors []error) {
	resultsChan, err := startGatheringConcurrently(ctx, gatherer, gatherConfig)
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

func readMemoryUsage() (int, error) {
	b, err := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if err != nil {
		return 0, err
	}
	memUsage := strings.ReplaceAll(string(b), "\n", "")
	return strconv.Atoi(memUsage)
}

// RecordArchiveMetadata records info about archive and gatherers' reports
func RecordArchiveMetadata(
	functionReports []GathererFunctionReport,
	rec recorder.Interface,
	anonymizer *anonymization.Anonymizer,
) error {
	memUsage, err := readMemoryUsage()
	if err != nil {
		klog.Warningf("can't read cgroups memory usage data: %v", err)
	}

	archiveMetadata := record.Record{
		Name: recorder.MetadataRecordName,
		Item: record.JSONMarshaller{Object: ArchiveMetadata{
			StatusReports:              functionReports,
			MemoryBytesUsage:           uint64(memUsage),
			Uptime:                     time.Since(programStartTime).Truncate(time.Millisecond).Seconds(),
			IsGlobalObfuscationEnabled: anonymizer.IsObfuscationEnabled(),
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
	ctx context.Context, gatherer gatherers.Interface, gatheringConfig *v1alpha1.GatherConfig,
) (chan GatheringFunctionResult, error) {
	var tasks []Task
	var gatheringFunctions map[string]gatherers.GatheringClosure
	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	if err != nil {
		return nil, err
	}

	// This is from TechPreview feature so we have to check the nil
	if gatheringConfig != nil {
		gatheringFunctions = getEnabledGatheringFunctions(gatherer.GetName(), gatheringFunctions, gatheringConfig.DisabledGatherers)
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
func getEnabledGatheringFunctions(gathererName string,
	allGatheringFunctions map[string]gatherers.GatheringClosure,
	disabledFunctions []string) map[string]gatherers.GatheringClosure {
	enabledGatheringFunctions := make(map[string]gatherers.GatheringClosure)

	// disabling a complete gatherer - e.g workloads
	if utils.StringInSlice(gathererName, disabledFunctions) {
		klog.Infof("%s gatherer is completely disabled", gathererName)
		return enabledGatheringFunctions
	}

	for fName, gatherinClosure := range allGatheringFunctions {
		fullGathererName := fmt.Sprintf("%s/%s", gathererName, fName)
		if !utils.StringInSlice(fullGathererName, disabledFunctions) {
			enabledGatheringFunctions[fName] = gatherinClosure
		}
	}
	return enabledGatheringFunctions
}
