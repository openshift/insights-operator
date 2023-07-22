package controller

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	insightClient, err := insightsv1alpha1cli.NewForConfig(kubeConfig)
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
	dataGatherCR, err := insightClient.DataGathers().Get(ctx, os.Getenv("DATAGATHER_NAME"), metav1.GetOptions{})
	if err != nil {
		klog.Error("failed to get coresponding DataGather custom resource: %v", err)
		return err
	}

	dataGatherCR, err = updateDataGatherStatus(ctx, *insightClient, dataGatherCR.DeepCopy(), insightsv1alpha1.Pending, nil)
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
	insightsClient := insightsclient.New(nil, 0, "default", authorizer, configClient)

	gatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configObserver, insightsClient,
	)
	uploader := insightsuploader.New(nil, insightsClient, configObserver, nil, nil, 0)

	dataGatherCR, err = updateDataGatherStatus(ctx, *insightClient, dataGatherCR, insightsv1alpha1.Running, nil)
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
		_, recErr := updateDataGatherStatus(ctx, *insightClient, dataGatherCR, insightsv1alpha1.Failed, conditions)
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
		_, updateErr := updateDataGatherStatus(ctx, *insightClient, dataGatherCR, insightsv1alpha1.Failed, conditions)
		if updateErr != nil {
			klog.Error("data upload failed and the update of DataGaher resource status failed as well: %v", updateErr)
		}
		return err
	}
	klog.Infof("Insights archive successfully uploaded with InsightsRequestID: %s", insightsRequestID)

	dataGatherCR.Status.InsightsRequestID = insightsRequestID
	conditions = append(conditions, status.DataUploadedCondition(metav1.ConditionTrue, reason, ""))

	_, err = updateDataGatherStatus(ctx, *insightClient, dataGatherCR, insightsv1alpha1.Completed, conditions)
	if err != nil {
		klog.Error(err)
		return err
	}
	// TODO use the InsightsRequestID to query the new aggregator API
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

// updateDataGatherStatus updates status' time attributes, state and conditions
// of the provided DataGather resource
func updateDataGatherStatus(ctx context.Context,
	insightsClient insightsv1alpha1cli.InsightsV1alpha1Client,
	dataGatherCR *insightsv1alpha1.DataGather,
	newState insightsv1alpha1.DataGatherState, conditions []metav1.Condition) (*insightsv1alpha1.DataGather, error) {
	switch newState {
	case insightsv1alpha1.Completed:
		dataGatherCR.Status.FinishTime = metav1.Now()
	case insightsv1alpha1.Failed:
		dataGatherCR.Status.FinishTime = metav1.Now()
	case insightsv1alpha1.Running:
		dataGatherCR.Status.StartTime = metav1.Now()
	case insightsv1alpha1.Pending:
		// no op
	}
	dataGatherCR.Status.State = newState
	if conditions != nil {
		dataGatherCR.Status.Conditions = append(dataGatherCR.Status.Conditions, conditions...)
	}
	return insightsClient.DataGathers().UpdateStatus(ctx, dataGatherCR, metav1.UpdateOptions{})
}
