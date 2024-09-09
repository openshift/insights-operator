package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/api/features"
	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	insightsv1alpha1cli "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1alpha1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/gatherers/conditional"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsuploader"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
)

// numberOfStatusQueryRetries is the number of attempts to query the processing status endpoint
// for particular archive/Insights request ID
var numberOfStatusQueryRetries = 3

// GatherJob is the type responsible for controlling a non-periodic Gather execution
type GatherJob struct {
	config.Controller
	InsightsConfigAPIEnabled bool
}

// processingStatusClient is an interface to call the "processingStatusEndpoint" in
// the "insights-results-aggregator" service running in console.redhat.com
type processingStatusClient interface {
	GetWithPathParam(ctx context.Context, endpoint, requestID string, includeClusterID bool) (*http.Response, error)
}

// Gather runs a single gather and stores the generated archive, without uploading it.
// 1. Creates the necessary configs/clients
// 2. Creates the configobserver to get more configs
// 3. Initiates the recorder
// 4. Executes a Gather
// 5. Flushes the results
func (g *GatherJob) Gather(ctx context.Context, kubeConfig, protoKubeConfig *rest.Config) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())
	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return err
	}

	gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig := prepareGatherConfigs(
		protoKubeConfig, kubeConfig, g.Impersonate,
	)

	// ensure the insight snapshot directory exists
	err = g.storagePathExists()
	if err != nil {
		return err
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(g.Controller, kubeClient)
	configAggregator := configobserver.NewStaticConfigAggregator(configObserver, kubeClient)

	// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
	anonymizer, err := anonymization.NewAnonymizerFromConfig(
		ctx, gatherKubeConfig, gatherProtoKubeConfig, protoKubeConfig, configAggregator, "")
	if err != nil {
		return err
	}

	// the recorder stores the collected data and we flush at the end.
	recdriver := diskrecorder.New(g.StoragePath)
	rec := recorder.New(recdriver, g.Interval, anonymizer)
	defer func() {
		if err = rec.Flush(); err != nil {
			klog.Error(err)
		}
	}()

	authorizer := clusterauthorizer.New(configObserver, configAggregator)

	// gatherConfigClient is configClient created from gatherKubeConfig, this name was used because configClient was already taken
	// this client is only used in insightsClient, it is created here
	// because pkg/insights/insightsclient/request_test.go unit test won't work otherwise
	gatherConfigClient, err := configv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return err
	}

	insightsClient := insightsclient.New(nil, 0, "default", authorizer, gatherConfigClient)
	createdGatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configAggregator, insightsClient, false,
	)

	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	for _, gatherer := range createdGatherers {
		functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, rec, nil)
		if err != nil {
			klog.Errorf("unable to process gatherer %v, error: %v", gatherer.GetName(), err)
		}

		for i := range functionReports {
			allFunctionReports[functionReports[i].FuncName] = functionReports[i]
		}
	}

	return gather.RecordArchiveMetadata(gather.FunctionReportsMapToArray(allFunctionReports), rec, anonymizer)
}

// GatherAndUpload runs a single gather and stores the generated archive, uploads it.
// 1. Prepare the necessary kube configs
// 2. Get the corresponding "datagathers.insights.openshift.io" resource
// 3. Create all the gatherers
// 4. Run data gathering
// 5. Recodrd the data into the Insights archive
// 6. Get the latest archive and upload it
// 7. Updates the status of the corresponding "datagathers.insights.openshift.io" resource continuously
func (g *GatherJob) GatherAndUpload(kubeConfig, protoKubeConfig *rest.Config) error { // nolint: funlen, gocyclo
	klog.Info("Starting data gathering")
	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return err
	}

	insightsV1alphaCli, err := insightsv1alpha1cli.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig := prepareGatherConfigs(
		protoKubeConfig, kubeConfig, g.Impersonate,
	)

	// The reason for using longer context is that the upload can fail and then there is the exponential backoff
	// See the insightsuploader Upload method
	ctx, cancel := context.WithTimeout(context.Background(), g.Interval*4)
	defer cancel()
	dataGatherCR, err := insightsV1alphaCli.DataGathers().Get(ctx, os.Getenv("DATAGATHER_NAME"), metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to get coresponding DataGather custom resource: %v", err)
		return err
	}
	// ensure the insight snapshot directory exists
	err = g.storagePathExists()
	if err != nil {
		return err
	}

	featureGate, err := featureGateAcces(ctx, kubeConfig)
	if err != nil {
		return err
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(g.Controller, kubeClient)
	configAggregator := configobserver.NewStaticConfigAggregator(configObserver, kubeClient)
	// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
	anonymizer, err := anonymization.NewAnonymizerFromConfig(
		ctx, gatherKubeConfig, gatherProtoKubeConfig, protoKubeConfig, configAggregator, dataGatherCR.Spec.DataPolicy)
	if err != nil {
		return err
	}

	// the recorder stores the collected data and we flush at the end.
	recdriver := diskrecorder.New(g.StoragePath)
	rec := recorder.New(recdriver, g.Interval, anonymizer)
	authorizer := clusterauthorizer.New(configObserver, configAggregator)

	configClient, err := configv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return err
	}
	insightsHTTPCli := insightsclient.New(nil, 0, "default", authorizer, configClient)

	createdGatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configAggregator, insightsHTTPCli, featureGate.Enabled(features.FeatureGateInsightsRuntimeExtractor),
	)
	uploader := insightsuploader.New(nil, insightsHTTPCli, configAggregator, nil, nil, 0)

	dataGatherCR, err = status.UpdateDataGatherState(ctx, insightsV1alphaCli, dataGatherCR, insightsv1alpha1.Running)
	if err != nil {
		klog.Errorf("failed to update coresponding DataGather custom resource: %v", err)
		return err
	}

	remoteConfigAvailableCondition := status.RemoteConfigurationNotAvailableCondition(metav1.ConditionTrue, status.AsExpectedReason, "")
	remoteConfigValidCondition := status.RemoteConfigurationInvalidCondition(metav1.ConditionUnknown, status.NoValidationYet, "")
	allFunctionReports, errsMap := gatherAndReporFunctions(ctx, createdGatherers, dataGatherCR, rec)
	if len(errsMap) > 0 {
		var remoteConfigErr conditional.RemoteConfigError
		conditionalErr := errsMap["conditional"]
		if errors.As(conditionalErr, &remoteConfigErr) {
			if remoteConfigErr.Reason == conditional.Invalid {
				remoteConfigValidCondition.Status = metav1.ConditionFalse
				remoteConfigValidCondition.Reason = remoteConfigErr.Reason
				remoteConfigValidCondition.Message = remoteConfigErr.Error()
			} else {
				remoteConfigAvailableCondition.Status = metav1.ConditionFalse
				remoteConfigAvailableCondition.Reason = remoteConfigErr.Reason
				remoteConfigAvailableCondition.Message = remoteConfigErr.Error()
			}
			rec.Record(record.Record{
				Name:         "insights-operator/remote-configuration.json",
				Item:         marshal.RawByte(remoteConfigErr.ConfigData),
				AlwaysStored: true,
			})
		} else {
			remoteConfigValidCondition.Status = metav1.ConditionTrue
			remoteConfigValidCondition.Reason = "AsExpected"
		}
	}
	dataGatherCR, err = status.UpdateDataGatherConditions(ctx, insightsV1alphaCli, dataGatherCR, &remoteConfigAvailableCondition)
	if err != nil {
		klog.Error(err)
	}
	dataGatherCR, err = status.UpdateDataGatherConditions(ctx, insightsV1alphaCli, dataGatherCR, &remoteConfigValidCondition)
	if err != nil {
		klog.Error(err)
	}

	// record data
	dataRecordedCon := status.DataRecordedCondition(metav1.ConditionTrue, "AsExpected", "")
	lastArchive, err := recordAllData(gather.FunctionReportsMapToArray(allFunctionReports), rec, recdriver, anonymizer)
	if err != nil {
		klog.Errorf("Failed to record data archive: %v", err)
		dataRecordedCon.Status = metav1.ConditionFalse
		dataRecordedCon.Reason = "RecordingFailed"
		dataRecordedCon.Message = fmt.Sprintf("Failed to record data: %v", err)
		updateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, &dataRecordedCon, insightsv1alpha1.Failed)
		return err
	}

	dataGatherCR, err = status.UpdateDataGatherConditions(ctx, insightsV1alphaCli, dataGatherCR, &dataRecordedCon)
	if err != nil {
		klog.Error(err)
	}

	// upload data
	insightsRequestID, statusCode, err := uploader.Upload(ctx, lastArchive)
	reason := fmt.Sprintf("HttpStatus%d", statusCode)
	dataUploadedCon := status.DataUploadedCondition(metav1.ConditionTrue, reason, "")
	if err != nil {
		klog.Errorf("Failed to upload data archive: %v", err)
		dataUploadedCon.Status = metav1.ConditionFalse
		dataUploadedCon.Reason = reason
		dataUploadedCon.Message = fmt.Sprintf("Failed to upload data: %v", err)
		updateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, &dataUploadedCon, insightsv1alpha1.Failed)
		return err
	}
	klog.Infof("Insights archive successfully uploaded with InsightsRequestID: %s", insightsRequestID)

	dataGatherCR.Status.InsightsRequestID = insightsRequestID
	dataGatherCR, err = status.UpdateDataGatherConditions(ctx, insightsV1alphaCli, dataGatherCR, &dataUploadedCon)
	if err != nil {
		klog.Error(err)
	}

	// check if the archive/data was processed
	processed, err := wasDataProcessed(ctx, insightsHTTPCli, insightsRequestID, configAggregator.Config())
	dataProcessedCon := status.DataProcessedCondition(metav1.ConditionTrue, "Processed", "")
	if err != nil || !processed {
		msg := fmt.Sprintf("Data was not processed in the console.redhat.com pipeline for the request %s", insightsRequestID)
		if err != nil {
			msg = fmt.Sprintf("%s: %v", msg, err)
		}
		klog.Info(msg)
		dataProcessedCon.Status = metav1.ConditionFalse
		dataProcessedCon.Reason = "Failure"
		dataProcessedCon.Message = fmt.Sprintf("failed to process data in the given time: %v", err)
		updateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, &dataProcessedCon, insightsv1alpha1.Failed)
		return err
	}
	updateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, &dataProcessedCon, insightsv1alpha1.Completed)
	klog.Infof("Data was successfully processed. New Insights analysis for the request ID %s will be downloaded by the operator",
		insightsRequestID)
	return nil
}

// gatherAndReporFunctions calls all the defined gatherers, calculates their status and returns map of resulting
// gatherer functions reports
func gatherAndReporFunctions(ctx context.Context, gatherersToRun []gatherers.Interface, // nolint: gocritic
	dataGatherCR *insightsv1alpha1.DataGather, rec *recorder.Recorder) (map[string]gather.GathererFunctionReport, map[string]error) {
	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	errsMap := make(map[string]error)
	for _, gatherer := range gatherersToRun {
		functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, rec, dataGatherCR.Spec.Gatherers) // nolint: govet
		if err != nil {
			klog.Errorf("unable to process gatherer %v, error: %v", gatherer.GetName(), err)
			errsMap[gatherer.GetName()] = err
		}

		for i := range functionReports {
			allFunctionReports[functionReports[i].FuncName] = functionReports[i]
		}
	}

	for k := range allFunctionReports {
		fr := allFunctionReports[k]
		// duration = 0 means the gatherer didn't run
		if fr.Duration == 0 {
			continue
		}

		gs := status.CreateDataGatherGathererStatus(&fr)
		dataGatherCR.Status.Gatherers = append(dataGatherCR.Status.Gatherers, gs)
	}
	return allFunctionReports, errsMap
}

// updateDataGatherStatus updates DataGather status conditions with provided condition definition as well as
// the DataGather state
func updateDataGatherStatus(ctx context.Context, insightsClient insightsv1alpha1cli.InsightsV1alpha1Interface,
	dataGatherCR *insightsv1alpha1.DataGather, conditionToUpdate *metav1.Condition, state insightsv1alpha1.DataGatherState) {
	dataGatherUpdated, err := status.UpdateDataGatherState(ctx, insightsClient, dataGatherCR, state)
	if err != nil {
		klog.Errorf("Failed to update DataGather resource %s state: %v", dataGatherCR.Name, err)
	}

	_, err = status.UpdateDataGatherConditions(ctx, insightsClient, dataGatherUpdated, conditionToUpdate)
	if err != nil {
		klog.Errorf("Failed to update DataGather resource %s conditions: %v", dataGatherCR.Name, err)
	}
}

// recordAllData is a helper function recording the archive metadata as well as data.
// Returns last known Insights archive and an error when recording failed.
func recordAllData(functionReports []gather.GathererFunctionReport,
	rec *recorder.Recorder, recdriver *diskrecorder.DiskRecorder, anonymizer *anonymization.Anonymizer) (*insightsclient.Source, error) {
	err := gather.RecordArchiveMetadata(functionReports, rec, anonymizer)
	if err != nil {
		return nil, err
	}
	err = rec.Flush()
	if err != nil {
		return nil, err
	}
	return recdriver.LastArchive()
}

// dataStatus is a helper struct to unmarshall
// the HTTP response from the processing status endpoint
type dataStatus struct {
	ClusterID string `json:"cluster"`
	Status    string `json:"status"`
}

// wasDataProcessed polls the "insights-results-aggregator" service processing status endpoint using provided
// "insightsRequestID" and tries to parse the response body in case of HTTP 200 response.
func wasDataProcessed(ctx context.Context,
	insightsCli processingStatusClient,
	insightsRequestID string, conf *config.InsightsConfiguration) (bool, error) {
	delay := conf.DataReporting.ReportPullingDelay
	retryCounter := 0
	klog.Infof("Initial delay when checking processing status: %v", delay)

	var resp *http.Response
	err := wait.PollUntilContextCancel(ctx, delay, false, func(ctx context.Context) (done bool, err error) {
		resp, err = insightsCli.GetWithPathParam(ctx, // nolint: bodyclose
			conf.DataReporting.ProcessingStatusEndpoint, insightsRequestID, true) // response body is closed later
		if err != nil {
			return false, err
		}
		if resp.StatusCode != http.StatusOK {
			if retryCounter == numberOfStatusQueryRetries {
				err := fmt.Errorf("HTTP status message: %s", http.StatusText(resp.StatusCode))
				return false, err
			}
			klog.Infof("Received HTTP status code %d, trying again in %s", resp.StatusCode, delay)
			retryCounter++
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		return false, err
	}

	if resp.Body == nil || resp.Body == http.NoBody {
		return false, nil
	}

	data, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return false, err
	}
	var processingResp dataStatus
	err = json.Unmarshal(data, &processingResp)
	if err != nil {
		return false, err
	}

	return processingResp.Status == "processed", nil
}

// storagePathExists checks if the configured storagePath exists or not.
// If not, non-nill error is returned.
func (g *GatherJob) storagePathExists() error {
	if _, err := os.Stat(g.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(g.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}
	return nil
}

func featureGateAcces(ctx context.Context, kubeConfig *rest.Config) (featuregates.FeatureGate, error) {
	configClient, err := configv1client.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	fg, err := configClient.ConfigV1().FeatureGates().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	fgAccess, err := featuregates.NewHardcodedFeatureGateAccessFromFeatureGate(fg, os.Getenv("RELEASE_VERSION"))
	if err != nil {
		return nil, err
	}

	return fgAccess.CurrentFeatureGates()
}
