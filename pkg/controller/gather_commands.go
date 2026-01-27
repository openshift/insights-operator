package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	insightsv1 "github.com/openshift/api/insights/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"

	insightsv1client "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsuploader"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

const (
	// numberOfStatusQueryRetries is the number of attempts to query the processing status endpoint for particular archive/Insights request ID
	numberOfStatusQueryRetries = 3

	// maxGatherJobArchives is the number of archives to keep on disk
	maxGatherJobArchives = 5
)

// GatherJob is the type responsible for controlling a non-periodic Gather execution
type GatherJob struct {
	config.Controller
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

	networkAnonymizer, err := anonymization.NewNetworkAnonymizerFromConfig(
		ctx, gatherKubeConfig, gatherProtoKubeConfig, protoKubeConfig, configAggregator, []insightsv1.DataPolicyOption{},
	)
	if err != nil {
		return err
	}

	// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
	anonymizer, err := anonymization.NewAnonymizer(networkAnonymizer)
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
		configAggregator, insightsClient,
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
// 5. Record the data into the Insights archive
// 6. Get the latest archive and upload it
// 7. Updates the status of the corresponding "datagathers.insights.openshift.io" resource continuously
func (g *GatherJob) GatherAndUpload(kubeConfig, protoKubeConfig *rest.Config) error { // nolint: funlen, gocyclo
	klog.Info("Starting data gathering")

	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return err
	}

	insightsV1Cli, err := insightsv1client.NewForConfig(kubeConfig)
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
	dataGatherCR, err := insightsV1Cli.DataGathers().Get(ctx, os.Getenv("DATAGATHER_NAME"), metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to get corresponding DataGather custom resource: %v", err)
		return err
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(g.Controller, kubeClient)
	configAggregator := configobserver.NewStaticConfigAggregator(configObserver, kubeClient)

	// additional configurations may exist besides the default one
	if customPath := getCustomStoragePath(configAggregator, dataGatherCR); customPath != "" {
		g.StoragePath = customPath
	}

	// if the dataGather uses persistenVolume, check if the volumePath was defined
	if dataGatherCR.Spec.Storage.Type == insightsv1.StorageTypePersistentVolume {
		if storagePath := dataGatherCR.Spec.Storage.PersistentVolume.MountPath; storagePath != "" {
			g.StoragePath = storagePath
		}
	}

	// ensure the insight snapshot directory exists
	err = g.storagePathExists()
	if err != nil {
		return err
	}

	networkAnonymizer, err := anonymization.NewNetworkAnonymizerFromConfig(
		ctx, gatherKubeConfig, gatherProtoKubeConfig, protoKubeConfig, configAggregator, dataGatherCR.Spec.DataPolicy,
	)
	if err != nil {
		return err
	}

	// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
	anonymizer, err := anonymization.NewAnonymizer(networkAnonymizer)
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
		configAggregator, insightsHTTPCli)
	uploader := insightsuploader.New(nil, insightsHTTPCli, configAggregator, nil, nil, 0)

	dataGatherCR, err = status.UpdateProgressingCondition(ctx, insightsV1Cli, dataGatherCR, dataGatherCR.Name, status.GatheringReason)
	if err != nil {
		klog.Errorf("failed to update corresponding DataGather custom resource: %v", err)
		return err
	}

	allFunctionReports, remoteConfStatus, err := gatherAndReportFunctions(ctx, createdGatherers, dataGatherCR, rec)
	if err != nil {
		klog.Errorf("failed to gatherAndReportFunctions: %v", err)
		return err
	}

	if remoteConfStatus != nil {
		rec.Record(record.Record{
			Name:         "insights-operator/remote-configuration.json",
			Item:         marshal.RawByte(remoteConfStatus.ConfigData),
			AlwaysStored: true,
		})
	}

	remoteConfigAvailableCondition, remoteConfigValidCondition := createRemoteConfigConditions(remoteConfStatus)
	dataGatherCR, err = status.UpdateDataGatherConditions(
		ctx, insightsV1Cli, dataGatherCR,
		remoteConfigAvailableCondition,
		remoteConfigValidCondition,
	)
	if err != nil {
		klog.Error(err)
	}

	// record data
	dataRecordedCondition := status.DataRecordedCondition(metav1.ConditionTrue, status.SucceededReason, "")
	lastArchive, err := recordAllData(gather.FunctionReportsMapToArray(allFunctionReports), rec, recdriver, anonymizer)
	if err != nil {
		klog.Errorf("Failed to record data archive: %v", err)
		dataRecordedCondition.Status = metav1.ConditionFalse
		dataRecordedCondition.Reason = status.RecordingFailedReason
		dataRecordedCondition.Message = fmt.Sprintf("Failed to record data: %v", err)
		updateDataGatherStatus(ctx, insightsV1Cli, dataGatherCR, &dataRecordedCondition, status.GatheringFailedReason)
		return err
	}

	dataGatherCR, err = status.UpdateDataGatherConditions(ctx, insightsV1Cli, dataGatherCR, dataRecordedCondition)
	if err != nil {
		klog.Error(err)
	}

	// upload data
	insightsRequestID, statusCode, err := uploader.Upload(ctx, lastArchive)
	dataUploadedCon := status.DataUploadedCondition(
		metav1.ConditionTrue,
		status.SucceededReason,
		fmt.Sprintf("Succeeded with http status code: %d", statusCode),
	)

	if err != nil {
		klog.Errorf("Failed to upload data archive: %v", err)
		dataUploadedCon.Status = metav1.ConditionFalse
		dataUploadedCon.Reason = status.FailedReason
		dataUploadedCon.Message = fmt.Sprintf("Failed to upload data err: %v with http status code: %d", err, statusCode)
		updateDataGatherStatus(ctx, insightsV1Cli, dataGatherCR, &dataUploadedCon, status.GatheringFailedReason)
		return err
	}
	klog.Infof("Insights archive successfully uploaded with InsightsRequestID: %s", insightsRequestID)

	dataGatherCR.Status.InsightsRequestID = insightsRequestID
	dataGatherCR, err = status.UpdateDataGatherConditions(ctx, insightsV1Cli, dataGatherCR, dataUploadedCon)
	if err != nil {
		klog.Error(err)
	}

	// check if the archive/data was processed
	processed, err := wasDataProcessed(ctx, insightsHTTPCli, insightsRequestID, configAggregator.Config())
	dataProcessedCondition := status.DataProcessedCondition(metav1.ConditionTrue, status.ProcessedReason, "")
	if err != nil || !processed {
		msg := fmt.Sprintf("Data was not processed in the console.redhat.com pipeline for the request %s", insightsRequestID)
		if err != nil {
			msg = fmt.Sprintf("%s: %v", msg, err)
		}
		klog.Info(msg)
		dataProcessedCondition.Status = metav1.ConditionFalse
		dataProcessedCondition.Reason = status.FailedReason
		dataProcessedCondition.Message = fmt.Sprintf("failed to process data in the given time: %v", err)
		updateDataGatherStatus(ctx, insightsV1Cli, dataGatherCR, &dataProcessedCondition, status.GatheringFailedReason)
		return err
	}

	updateDataGatherStatus(ctx, insightsV1Cli, dataGatherCR, &dataProcessedCondition, status.GatheringSucceededReason)
	klog.Infof(
		"Data was successfully processed. New Insights analysis for the request ID %s will be downloaded by the operator",
		insightsRequestID,
	)

	// Clean up of old archives created by on-demand gathering
	if err := recdriver.PruneByCount(maxGatherJobArchives); err != nil {
		klog.Errorf("Failed to prune archives: %v", err)
	}

	return nil
}

// gatherAndReportFunctions calls all the defined gatherers, calculates their status and returns map of resulting
// gatherer functions reports
func gatherAndReportFunctions(
	ctx context.Context,
	gatherersToRun []gatherers.Interface, // nolint: gocritic
	dataGatherCR *insightsv1.DataGather,
	rec *recorder.Recorder,
) (
	map[string]gather.GathererFunctionReport,
	*gatherers.RemoteConfigStatus,
	error,
) {
	if dataGatherCR == nil {
		return nil, nil, fmt.Errorf("failed to to gather: datagather resource is nil")
	}

	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	var remoteConfStatus gatherers.RemoteConfigStatus

	var gatheringConfig []insightsv1.GathererConfig

	// Check if custom config should be used
	if dataGatherCR.Spec.Gatherers.Mode == insightsv1.GatheringModeCustom {
		gatheringConfig = dataGatherCR.Spec.Gatherers.Custom.Configs
	}

	for _, gatherer := range gatherersToRun {
		functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, rec, gatheringConfig) // nolint: govet
		if err != nil {
			klog.Errorf("unable to process gatherer %v, error: %v", gatherer.GetName(), err)
		}

		for i := range functionReports {
			allFunctionReports[functionReports[i].FuncName] = functionReports[i]
		}

		if gathererUsingRemoteConf, ok := gatherer.(gatherers.GathererUsingRemoteConfig); ok {
			remoteConfStatus = gathererUsingRemoteConf.RemoteConfigStatus()
		}
	}

	for k := range allFunctionReports {
		fr := allFunctionReports[k]
		// duration = 0 means the gatherer didn't run
		if fr.Duration == 0 {
			continue
		}

		gs, err := status.CreateDataGatherGathererStatus(&fr)
		if err != nil {
			return nil, nil, err
		}

		dataGatherCR.Status.Gatherers = append(dataGatherCR.Status.Gatherers, *gs)
	}
	return allFunctionReports, &remoteConfStatus, nil
}

// updateDataGatherStatus updates DataGather status conditions with provided condition definition as well as
// the DataGather state
func updateDataGatherStatus(
	ctx context.Context,
	insightsClient insightsv1client.InsightsV1Interface,
	dataGatherCR *insightsv1.DataGather,
	conditionToUpdate *metav1.Condition,
	gatheringStatus string,
) {
	if dataGatherCR == nil {
		klog.Errorf("cannot update DataGather status: resource is nil")
		return
	}

	if conditionToUpdate == nil {
		klog.Errorf("cannot update DataGather conditions: condition is nil")
		return
	}

	dataGatherUpdated, err := status.UpdateProgressingCondition(ctx, insightsClient, dataGatherCR, dataGatherCR.Name, gatheringStatus)
	if err != nil {
		klog.Errorf("Failed to update DataGather resource %s state: %v", dataGatherCR.Name, err)
	}

	_, err = status.UpdateDataGatherConditions(
		ctx,
		insightsClient,
		dataGatherUpdated,
		status.ProgressingCondition(gatheringStatus),
		*conditionToUpdate,
	)
	if err != nil {
		klog.Errorf("Failed to update DataGather resource %s conditions: %v", dataGatherCR.Name, err)
	}
}

// recordAllData is a helper function recording the archive metadata as well as data.
// Returns last known Insights archive and an error when recording failed.
func recordAllData(functionReports []gather.GathererFunctionReport,
	rec *recorder.Recorder, recdriver *diskrecorder.DiskRecorder, anonymizer *anonymization.Anonymizer,
) (*insightsclient.Source, error) {
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

// retryCounter is a helper struct to store number of attempts
// for each failure type when processing data
type retryCounter struct {
	network int
	request int
	status  int

	// maximum retry attempts allowed for each failure type before failing completely
	max int
}

// wasDataProcessed polls the "insights-results-aggregator" service processing status endpoint using provided
// "insightsRequestID" and retries up to numberOfStatusQueryRetries times for network errors and HTTP errors independently,
// or until the context expires. Returns true if the data was successfully processed, false otherwise.
func wasDataProcessed(ctx context.Context,
	insightsCli processingStatusClient,
	insightsRequestID string, conf *config.InsightsConfiguration,
) (bool, error) {
	delay := conf.DataReporting.ReportPullingDelay
	klog.Infof("Initial delay when checking processing status: %v", delay)

	retryCounter := &retryCounter{max: numberOfStatusQueryRetries}

	err := wait.PollUntilContextCancel(ctx, delay, false, func(ctx context.Context) (done bool, err error) {
		resp, err := insightsCli.GetWithPathParam(
			ctx, conf.DataReporting.ProcessingStatusEndpoint, insightsRequestID, true,
		)
		// Handle network errors
		if err != nil {
			return false, networkRetry(retryCounter, err, delay)
		}

		defer resp.Body.Close()

		// Handle server errors
		if resp.StatusCode != http.StatusOK {
			return false, requestRetry(retryCounter, resp.StatusCode, delay)
		}

		return processSuccessfulResponse(resp.Body, retryCounter, delay)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

// networkRetry is used to retry when there is a network issue that caused failure
func networkRetry(retryCounter *retryCounter, err error, delay time.Duration) error {
	if retryCounter.network >= retryCounter.max {
		return fmt.Errorf("failed to check processing status after %d retries: %w", retryCounter.network, err)
	}
	klog.Infof("Network error when checking processing status: %v, retry %d/%d in %s",
		err, retryCounter.network+1, retryCounter.max, delay)
	retryCounter.network++
	return nil
}

// requestRetry is used to retry an http request when the response != 200
func requestRetry(retryCounter *retryCounter, respStatusCode int, delay time.Duration) error {
	if retryCounter.request >= retryCounter.max {
		return fmt.Errorf("HTTP status message: %s", http.StatusText(respStatusCode))
	}
	klog.Infof("Received HTTP status code %d, retry %d/%d in %s",
		respStatusCode, retryCounter.request+1, retryCounter.max, delay)
	retryCounter.request++
	return nil
}

// processSuccessfulResponse is used to process response body and if data is not processed, it retries 3 times
// Returns true if the data was successfully processed, false otherwise
func processSuccessfulResponse(respBody io.ReadCloser, retryCounter *retryCounter, delay time.Duration) (bool, error) {
	if respBody == nil || respBody == http.NoBody {
		return false, nil
	}

	data, err := io.ReadAll(respBody)
	if err != nil {
		return false, err
	}

	var processingResp dataStatus
	err = json.Unmarshal(data, &processingResp)
	if err != nil {
		return false, err
	}

	// If data is not processed yet, retry 3 times before failing
	if processingResp.Status != "processed" {
		return statusRetry(retryCounter, processingResp.Status, delay)
	}

	return true, nil
}

// statusRetry is used to retry when the processing pipeline is not finished yet
func statusRetry(retryCounter *retryCounter, processingRespStatus string, delay time.Duration) (bool, error) {
	if retryCounter.status >= retryCounter.max {
		klog.Infof("Data status is %q after %d retries, stopping poll", processingRespStatus, retryCounter.status)
		return false, fmt.Errorf("data processing status is %q after %d retries, stopping poll", processingRespStatus, retryCounter.status)
	}
	klog.Infof("Data status is %q, retry %d/%d in %s",
		processingRespStatus, retryCounter.status+1, retryCounter.max, delay)
	retryCounter.status++
	return false, nil
}

// storagePathExists checks if the configured storagePath exists or not.
// If not, non-nill error is returned.
func (g *GatherJob) storagePathExists() error {
	if _, err := os.Stat(g.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(g.StoragePath, 0o777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}
	return nil
}

// createRemoteConfigConditions create RemoteConfiguration conditions based on the provided RemoteConfigStatus
func createRemoteConfigConditions(
	remoteConfStatus *gatherers.RemoteConfigStatus,
) (remoteConfigAvailableCondition, remoteConfigValidCondition metav1.Condition) {
	remoteConfigAvailableCondition = status.RemoteConfigurationAvailableCondition(
		metav1.ConditionUnknown, status.RemoteConfNotRequestedYet, "",
	)
	remoteConfigValidCondition = status.RemoteConfigurationValidCondition(
		metav1.ConditionUnknown, status.RemoteConfNotValidatedYet, "",
	)

	if remoteConfStatus == nil {
		return
	}

	remoteConfigAvailableCondition.Status = boolToConditionStatus(remoteConfStatus.ConfigAvailable)
	remoteConfigAvailableCondition.Reason = remoteConfStatus.AvailableReason
	if !remoteConfStatus.ConfigAvailable {
		remoteConfigAvailableCondition.Message = remoteConfStatus.Err.Error()
	}

	// set the remoteConfigValidCondition only if the remoteConfig is available
	if remoteConfStatus.ConfigAvailable {
		remoteConfigValidCondition.Status = boolToConditionStatus(remoteConfStatus.ConfigValid)
		remoteConfigValidCondition.Reason = remoteConfStatus.ValidReason
		if !remoteConfStatus.ConfigValid {
			remoteConfigValidCondition.Message = remoteConfStatus.Err.Error()
		}
	}
	return
}

// boolToConditionStatus is a helper function to conver bool type
// tp the ConditionStatus type
func boolToConditionStatus(b bool) metav1.ConditionStatus {
	var conditionStatus metav1.ConditionStatus
	if b {
		conditionStatus = metav1.ConditionTrue
	} else {
		conditionStatus = metav1.ConditionFalse
	}
	return conditionStatus
}
