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

// GatherAndUpload runs a single gather and stores the generated archive, uploads it
// and waits for the corresponding Insights analysis report.
// 1. Creates the necessary configs/clients
// 2. Creates the configobserver
// 3. Initiates the recorder
// 4. Executes a Gather
// 5. Flushes the results
// 6. Get the latest archive
// 7. Uploads the archive
// 8. Waits for the corresponding Insights analysis download
func (d *GatherJob) GatherAndUpload(kubeConfig, protoKubeConfig *rest.Config) error { // nolint: funlen, gocyclo
	klog.Infof("Starting insights-operator %s", version.Get().String())
	// these are operator clients
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
	updatedCR := dataGatherCR.DeepCopy()
	updatedCR.Status.State = insightsv1alpha1.Running
	updatedCR.Status.StartTime = metav1.Now()

	dataGatherCR, err = insightClient.DataGathers().UpdateStatus(ctx, updatedCR, metav1.UpdateOptions{})
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
	err = gather.RecordArchiveMetadata(mapToArray(allFunctionReports), rec, anonymizer)
	if err != nil {
		klog.Error(err)
		return err
	}
	err = rec.Flush()
	if err != nil {
		klog.Error(err)
		return err
	}
	lastArchive, err := recdriver.LastArchive()
	if err != nil {
		klog.Error(err)
		return err
	}
	insightsRequestID, err := uploader.Upload(ctx, lastArchive)
	if err != nil {
		klog.Error(err)
		return err
	}
	klog.Infof("Insights archive successfully uploaded with InsightsRequestID: %s", insightsRequestID)

	dataGatherCR.Status.FinishTime = metav1.Now()
	dataGatherCR.Status.State = insightsv1alpha1.Completed
	dataGatherCR.Status.InsightsRequestID = insightsRequestID
	dataGatherCR.Status.Conditions = []metav1.Condition{
		{
			Type:               "DataUploaded",
			Status:             metav1.ConditionTrue,
			Reason:             "AsExpected",
			LastTransitionTime: metav1.Now(),
		},
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
	_, err = insightClient.DataGathers().UpdateStatus(ctx, dataGatherCR, metav1.UpdateOptions{})
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
