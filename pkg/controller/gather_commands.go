package controller

import (
	"context"
	"encoding/json"
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

	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	insightsv1alpha1cli "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1alpha1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsuploader"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
)

// GatherJob is the type responsible for controlling a non-periodic Gather execution
type GatherJob struct {
	config.Controller
	InsightsConfigAPIEnabled bool
}

// processingStatusClient is an interface to call the "processingStatusEndpoint" in
// the "insights-results-aggregator" service running in console.redhat.com
type processingStatusClient interface {
	GetWithPathParams(ctx context.Context, endpoint, requestID string) (*http.Response, error)
}

// Gather runs a single gather and stores the generated archive, without uploading it.
// 1. Creates the necessary configs/clients
// 2. Creates the configobserver to get more configs
// 3. Initiates the recorder
// 4. Executes a Gather
// 5. Flushes the results
func (d *GatherJob) Gather(ctx context.Context, kubeConfig, protoKubeConfig *rest.Config) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())
	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return err
	}

	gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig := prepareGatherConfigs(
		protoKubeConfig, kubeConfig, d.Impersonate,
	)

	// ensure the insight snapshot directory exists
	if _, err = os.Stat(d.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(d.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(d.Controller, kubeClient)

	// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
	anonymizer, err := anonymization.NewAnonymizerFromConfig(
		ctx, gatherKubeConfig, gatherProtoKubeConfig, protoKubeConfig, configObserver, "")
	if err != nil {
		return err
	}

	// the recorder stores the collected data and we flush at the end.
	recdriver := diskrecorder.New(d.StoragePath)
	rec := recorder.New(recdriver, d.Interval, anonymizer)
	defer func() {
		if err = rec.Flush(); err != nil {
			klog.Error(err)
		}
	}()

	authorizer := clusterauthorizer.New(configObserver)

	// gatherConfigClient is configClient created from gatherKubeConfig, this name was used because configClient was already taken
	// this client is only used in insightsClient, it is created here
	// because pkg/insights/insightsclient/request_test.go unit test won't work otherwise
	gatherConfigClient, err := configv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return err
	}

	insightsClient := insightsclient.New(nil, 0, "default", authorizer, gatherConfigClient)
	gatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configObserver, insightsClient,
	)

	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	for _, gatherer := range gatherers {
		functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, rec, nil)
		if err != nil {
			klog.Errorf("unable to process gatherer %v, error: %v", gatherer.GetName(), err)
		}

		for i := range functionReports {
			allFunctionReports[functionReports[i].FuncName] = functionReports[i]
		}
	}

	return gather.RecordArchiveMetadata(mapToArray(allFunctionReports), rec, anonymizer)
}

// GatherAndUpload runs a single gather and stores the generated archive, uploads it.
// 1. Prepare the necessary kube configs
// 2. Get the corresponding "datagathers.insights.openshift.io" resource
// 3. Create all the gatherers
// 4. Run data gathering
// 5. Recodrd the data into the Insights archive
// 6. Get the latest archive and upload it
// 7. Updates the status of the corresponding "datagathers.insights.openshift.io" resource continuously
func (d *GatherJob) GatherAndUpload(kubeConfig, protoKubeConfig *rest.Config) error { // nolint: funlen, gocyclo
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
		protoKubeConfig, kubeConfig, d.Impersonate,
	)

	// The reason for using longer context is that the upload can fail and then there is the exponential backoff
	// See the insightsuploader Upload method
	ctx, cancel := context.WithTimeout(context.Background(), d.Interval*4)
	defer cancel()
	dataGatherCR, err := insightsV1alphaCli.DataGathers().Get(ctx, os.Getenv("DATAGATHER_NAME"), metav1.GetOptions{})
	if err != nil {
		klog.Error("failed to get coresponding DataGather custom resource: %v", err)
		return err
	}

	dataGatherCR, err = status.UpdateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR.DeepCopy(), insightsv1alpha1.Pending, nil)
	if err != nil {
		klog.Error("failed to update coresponding DataGather custom resource: %v", err)
		return err
	}

	// ensure the insight snapshot directory exists
	if _, err = os.Stat(d.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(d.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(d.Controller, kubeClient)
	// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
	anonymizer, err := anonymization.NewAnonymizerFromConfig(
		ctx, gatherKubeConfig, gatherProtoKubeConfig, protoKubeConfig, configObserver, dataGatherCR.Spec.DataPolicy)
	if err != nil {
		return err
	}

	// the recorder stores the collected data and we flush at the end.
	recdriver := diskrecorder.New(d.StoragePath)
	rec := recorder.New(recdriver, d.Interval, anonymizer)
	authorizer := clusterauthorizer.New(configObserver)

	configClient, err := configv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return err
	}
	insightsHTTPCli := insightsclient.New(nil, 0, "default", authorizer, configClient)

	gatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configObserver, insightsHTTPCli,
	)
	uploader := insightsuploader.New(nil, insightsHTTPCli, configObserver, nil, nil, 0)

	dataGatherCR, err = status.UpdateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, insightsv1alpha1.Running, nil)
	if err != nil {
		klog.Error("failed to update coresponding DataGather custom resource: %v", err)
		return err
	}
	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	for _, gatherer := range gatherers {
		functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, rec, dataGatherCR.Spec.Gatherers) // nolint: govet
		if err != nil {
			klog.Errorf("unable to process gatherer %v, error: %v", gatherer.GetName(), err)
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

	// record data
	conditions := []metav1.Condition{}
	lastArchive, err := record(mapToArray(allFunctionReports), rec, recdriver, anonymizer)
	if err != nil {
		conditions = append(conditions, status.DataRecordedCondition(metav1.ConditionFalse, "RecordingFailed",
			fmt.Sprintf("Failed to record data: %v", err)))
		_, recErr := status.UpdateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, insightsv1alpha1.Failed, conditions)
		if recErr != nil {
			klog.Error("data recording failed and the update of DataGaher resource status failed as well: %v", recErr)
		}
		return err
	}
	conditions = append(conditions, status.DataRecordedCondition(metav1.ConditionTrue, "AsExpected", ""))

	// upload data
	insightsRequestID, statusCode, err := uploader.Upload(ctx, lastArchive)
	reason := fmt.Sprintf("HttpStatus%d", statusCode)
	if err != nil {
		klog.Error(err)
		conditions = append(conditions, status.DataUploadedCondition(metav1.ConditionFalse, reason,
			fmt.Sprintf("Failed to upload data: %v", err)))
		_, updateErr := status.UpdateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, insightsv1alpha1.Failed, conditions)
		if updateErr != nil {
			klog.Error("data upload failed and the update of DataGaher resource status failed as well: %v", updateErr)
		}
		return err
	}
	klog.Infof("Insights archive successfully uploaded with InsightsRequestID: %s", insightsRequestID)

	dataGatherCR.Status.InsightsRequestID = insightsRequestID
	conditions = append(conditions, status.DataUploadedCondition(metav1.ConditionTrue, reason, ""))

	// check if the archive/data was processed
	processed, err := wasDataProcessed(ctx, insightsHTTPCli, insightsRequestID, configObserver.Config())
	if err != nil || !processed {
		klog.Error(err)
		conditions = append(conditions,
			status.DataProcessedCondition(metav1.ConditionFalse, "Failure", fmt.Sprintf("failed to process data in the given time: %v", err)))
	} else {
		conditions = append(conditions, status.DataProcessedCondition(metav1.ConditionTrue, "Processed", ""))
	}
	_, err = status.UpdateDataGatherStatus(ctx, insightsV1alphaCli, dataGatherCR, insightsv1alpha1.Completed, conditions)
	if err != nil {
		klog.Error(err)
		return err
	}
	return nil
}

func mapToArray(m map[string]gather.GathererFunctionReport) []gather.GathererFunctionReport {
	a := make([]gather.GathererFunctionReport, 0, len(m))
	for _, v := range m {
		a = append(a, v)
	}
	return a
}

// record is a helper function recording the archive metadata as well as data.
// Returns last known Insights archive and an error when recording failed.
func record(functionReports []gather.GathererFunctionReport,
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
	insightsRequestID string, controllerConf *config.Controller) (bool, error) {
	delay := controllerConf.ReportPullingDelay
	retryCounter := 0
	klog.V(4).Infof("Initial delay when checking processing status: %v", delay)

	var resp *http.Response
	err := wait.PollUntilContextCancel(ctx, delay, false, func(ctx context.Context) (done bool, err error) {
		resp, err = insightsCli.GetWithPathParams(ctx, // nolint: bodyclose
			controllerConf.ProcessingStatusEndpoint, insightsRequestID) // response body is closed later
		if err != nil {
			return false, err
		}
		if resp.StatusCode == http.StatusOK || retryCounter == 2 {
			return true, nil
		}
		klog.Infof("Received HTTP status code %d, trying again in %s", resp.StatusCode, delay)
		retryCounter++
		return false, nil
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
