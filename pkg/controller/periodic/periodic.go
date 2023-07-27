package periodic

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	configv1alpha1 "github.com/openshift/api/config/v1alpha1"
	insightsv1alpha1 "github.com/openshift/api/insights/v1alpha1"
	v1 "github.com/openshift/api/operator/v1"
	insightsv1alpha1cli "github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1alpha1"
	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/insights"
	"github.com/openshift/insights-operator/pkg/insights/insightsreport"
	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/openshift/insights-operator/pkg/recorder"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	serviceCABundle             = "service-ca-bundle"
	serviceCABundlePath         = "/var/run/configmaps/service-ca-bundle"
	insightsNamespace           = "openshift-insights"
	falseB                      = new(bool)
	trueB                       = true
	deletePropagationBackground = metav1.DeletePropagationBackground
)

// Controller periodically runs gatherers, records their results to the recorder
// and flushes the recorder to create archives
type Controller struct {
	secretConfigurator  configobserver.Configurator
	apiConfigurator     configobserver.InsightsDataGatherObserver
	recorder            recorder.FlushInterface
	gatherers           []gatherers.Interface
	statuses            map[string]controllerstatus.StatusController
	anonymizer          *anonymization.Anonymizer
	insightsOperatorCLI operatorv1client.InsightsOperatorInterface
	dataGatherClient    insightsv1alpha1cli.InsightsV1alpha1Interface
	kubeClient          kubernetes.Interface
	reportRetriever     *insightsreport.Controller
	image               string
	jobController       *JobController
	pruneInterval       time.Duration
	techPreview         bool
}

func NewWithTechPreview(
	reportRetriever *insightsreport.Controller,
	secretConfigurator configobserver.Configurator,
	apiConfigurator configobserver.InsightsDataGatherObserver,
	listGatherers []gatherers.Interface,
	kubeClient kubernetes.Interface,
	dataGatherClient insightsv1alpha1cli.InsightsV1alpha1Interface,
	insightsOperatorCLI operatorv1client.InsightsOperatorInterface,
) *Controller {
	statuses := make(map[string]controllerstatus.StatusController)

	statuses["insightsuploader"] = controllerstatus.New("insightsuploader")
	jobController := NewJobController(kubeClient)
	return &Controller{
		reportRetriever:     reportRetriever,
		secretConfigurator:  secretConfigurator,
		apiConfigurator:     apiConfigurator,
		gatherers:           listGatherers,
		statuses:            statuses,
		kubeClient:          kubeClient,
		dataGatherClient:    dataGatherClient,
		jobController:       jobController,
		insightsOperatorCLI: insightsOperatorCLI,
		pruneInterval:       1 * time.Hour,
		techPreview:         true,
	}
}

// New creates a new instance of Controller which periodically invokes the gatherers
// and flushes the recorder to create archives.
func New(
	secretConfigurator configobserver.Configurator,
	rec recorder.FlushInterface,
	listGatherers []gatherers.Interface,
	anonymizer *anonymization.Anonymizer,
	insightsOperatorCLI operatorv1client.InsightsOperatorInterface,
	kubeClient *kubernetes.Clientset,
) *Controller {
	statuses := make(map[string]controllerstatus.StatusController)

	for _, gatherer := range listGatherers {
		gathererName := gatherer.GetName()
		statuses[gathererName] = controllerstatus.New(fmt.Sprintf("periodic-%s", gathererName))
	}

	return &Controller{
		secretConfigurator:  secretConfigurator,
		recorder:            rec,
		gatherers:           listGatherers,
		statuses:            statuses,
		anonymizer:          anonymizer,
		insightsOperatorCLI: insightsOperatorCLI,
		kubeClient:          kubeClient,
	}
}

func (c *Controller) Sources() []controllerstatus.StatusController {
	keys := make([]string, 0, len(c.statuses))
	for key := range c.statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sources := make([]controllerstatus.StatusController, 0, len(keys))
	for _, key := range keys {
		sources = append(sources, c.statuses[key])
	}
	return sources
}

func (c *Controller) Run(stopCh <-chan struct{}, initialDelay time.Duration) {
	defer utilruntime.HandleCrash()
	defer klog.Info("Shutting down")

	// Runs a gather after startup
	if initialDelay > 0 {
		select {
		case <-stopCh:
			return
		case <-time.After(initialDelay):
			if c.techPreview {
				c.GatherJob()
			} else {
				c.Gather()
			}
		}
	} else {
		if c.techPreview {
			c.GatherJob()
		} else {
			c.Gather()
		}
	}

	go wait.Until(func() { c.periodicTrigger(stopCh) }, time.Second, stopCh)

	<-stopCh
}

// Gather Runs the gatherers one after the other.
func (c *Controller) Gather() {
	if c.isGatheringDisabled() {
		klog.V(3).Info("Gather is disabled by configuration.")
		return
	}

	// flush when all necessary gatherers were processed
	defer func() {
		if err := c.recorder.Flush(); err != nil {
			klog.Errorf("Unable to flush the recorder: %v", err)
		}
	}()

	var gatherersToProcess []gatherers.Interface

	for _, gatherer := range c.gatherers {
		if g, ok := gatherer.(gatherers.CustomPeriodGatherer); ok {
			if g.ShouldBeProcessedNow() {
				gatherersToProcess = append(gatherersToProcess, g)
				g.UpdateLastProcessingTime()
			}
		} else {
			gatherersToProcess = append(gatherersToProcess, gatherer)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.secretConfigurator.Config().Interval)
	defer cancel()

	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	gatherTime := metav1.Now()
	for _, gatherer := range gatherersToProcess {
		func() {
			name := gatherer.GetName()
			start := time.Now()

			klog.V(4).Infof("Running %s gatherer", gatherer.GetName())
			functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, c.recorder, nil)
			for i := range functionReports {
				allFunctionReports[functionReports[i].FuncName] = functionReports[i]
			}
			if err == nil {
				klog.V(3).Infof("Periodic gather %s completed in %s", name, time.Since(start).Truncate(time.Millisecond))
				c.statuses[name].UpdateStatus(controllerstatus.Summary{Healthy: true})
				return
			}

			utilruntime.HandleError(fmt.Errorf("%v failed after %s with: %v", name, time.Since(start).Truncate(time.Millisecond), err))
			c.statuses[name].UpdateStatus(controllerstatus.Summary{
				Operation: controllerstatus.GatheringReport,
				Reason:    "PeriodicGatherFailed",
				Message:   fmt.Sprintf("Source %s could not be retrieved: %v", name, err),
			})
		}()
	}
	err := c.updateOperatorStatusCR(ctx, allFunctionReports, gatherTime)
	if err != nil {
		klog.Errorf("failed to update the Insights Operator CR status: %v", err)
	}
	err = gather.RecordArchiveMetadata(mapToArray(allFunctionReports), c.recorder, c.anonymizer)
	if err != nil {
		klog.Errorf("unable to record archive metadata because of error: %v", err)
	}
}

// Periodically starts the gathering.
// If there is an initialDelay set then it waits that much for the first gather to happen.
func (c *Controller) periodicTrigger(stopCh <-chan struct{}) {
	configCh, closeFn := c.secretConfigurator.ConfigChanged()
	defer closeFn()

	interval := c.secretConfigurator.Config().Interval
	klog.Infof("Gathering cluster info every %s", interval)
	for {
		select {
		case <-stopCh:
			return

		case <-configCh:
			newInterval := c.secretConfigurator.Config().Interval
			if newInterval == interval {
				continue
			}
			interval = newInterval
			klog.Infof("Gathering cluster info every %s", interval)

		case <-time.After(interval):
			if c.techPreview {
				c.GatherJob()
			} else {
				c.Gather()
			}
		}
	}
}

func (c *Controller) GatherJob() {
	if c.isGatheringDisabled() {
		klog.V(3).Info("Gather is disabled by configuration.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.secretConfigurator.Config().Interval*4)
	defer cancel()

	if c.image == "" {
		image, err := c.getInsightsImage(ctx)
		if err != nil {
			klog.Errorf("Can't get operator image. Gathering will not run: %v", err)
			return
		}
		c.image = image
	}

	disabledGatherers, dp := c.createDataGatherAttributeValues()
	// create a new datagather.insights.openshift.io custom resource
	dataGatherCR, err := c.createNewDataGatherCR(ctx, disabledGatherers, dp)
	if err != nil {
		klog.Errorf("Failed to create a new DataGather resource: %v", err)
		return
	}

	// create a new periodic gathering job
	gj, err := c.jobController.CreateGathererJob(ctx, dataGatherCR.Name, c.image, c.secretConfigurator.Config().StoragePath)
	if err != nil {
		klog.Errorf("Failed to create a new job: %v", err)
		return
	}

	klog.Infof("Created new gathering job %v", gj.Name)
	err = c.jobController.WaitForJobCompletion(ctx, gj)
	if err != nil {
		if err == context.DeadlineExceeded {
			klog.Errorf("Failed to read job status: %v", err)
			return
		}
		klog.Error(err)
	}
	klog.Infof("Job completed %s", gj.Name)
	dataGatherFinished, err := c.dataGatherClient.DataGathers().Get(ctx, dataGatherCR.Name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get DataGather resource %s: %v", dataGatherCR.Name, err)
		return
	}
	dataUploaded := c.wasDataUploaded(dataGatherFinished)
	updateMetrics(dataGatherFinished)
	if !dataUploaded {
		klog.Errorf("Last data gathering %v was not successful", dataGatherFinished.Name)
		conditions := []metav1.Condition{
			status.DataRecordedCondition(metav1.ConditionFalse, "JobNotSucceeded", ""),
			status.DataUploadedCondition(metav1.ConditionFalse, "JobNotSucceeded", ""),
		}
		_, err = status.UpdateDataGatherStatus(ctx, c.dataGatherClient, dataGatherFinished, insightsv1alpha1.Failed, conditions)
		if err != nil {
			klog.Errorf("Failed to update failed DataGather resource %s: %v ", dataGatherFinished.Name, err)
		}
		return
	}

	// check data was processed in the external pipeline
	dataProcessed := c.wasDataProcessed(dataGatherFinished)
	if !dataProcessed {
		klog.Infof("Data archive for the %s was not processed in the console.redhat.com. New Insights analysis is not available.",
			dataGatherFinished.Name)
		return
	}

	// try to pull corresponding Insights analysis report
	analysisReport, err := c.reportRetriever.PullReportTechpreview(dataGatherFinished.Status.InsightsRequestID)
	if err != nil {
		klog.Errorf("Failed to download Insights analysis for the Insights request %s: %v", dataGatherFinished.Status.InsightsRequestID, err)
		return
	}

	// update Insights analysis report in the DataGather custom resource
	err = c.updateInsightsReportInDataGather(ctx, analysisReport, *dataGatherFinished)
	if err != nil {
		klog.Errorf("Failed to update Insights report in the %s custom resource:%v", dataGatherFinished.Name, err)
	}

	_, err = c.copyDataGatherStatusToOperatorStatus(ctx, dataGatherFinished)
	if err != nil {
		klog.Errorf("Failed to copy the last DataGather status to \"cluster\" operator status: %v", err)
		return
	}
	klog.Info("Operator status in \"insightsoperator.operator.openshift.io\" successfully updated")
}

// updateInsightsReportInDataGather reads the recommendations from the provided InsightsAnalysisReport and
// updates the provided DataGather resource with all important attributes.
func (c *Controller) updateInsightsReportInDataGather(ctx context.Context, report *types.InsightsAnalysisReport, dg insightsv1alpha1.DataGather) error {
	for _, recommendation := range report.Recommendations {
		healthCheck := insightsv1alpha1.HealthCheck{
			Description: recommendation.Description,
			TotalRisk:   int32(recommendation.TotalRisk),
			State:       insightsv1alpha1.HealthCheckEnabled,
			AdvisorURI: fmt.Sprintf("https://console.redhat.com/openshift/insights/advisor/clusters/%s?first=%s|%s",
				report.ClusterID, recommendation.RuleFQDN, recommendation.ErrorKey),
		}
		dg.Status.InsightsReport.HealthChecks = append(dg.Status.InsightsReport.HealthChecks, healthCheck)
	}
	dg.Status.InsightsReport.DownloadedAt = report.DownloadedAt
	uri := fmt.Sprintf(c.secretConfigurator.Config().ReportEndpointTechPreview, report.ClusterID, report.RequestID)
	dg.Status.InsightsReport.URI = uri
	_, err := c.dataGatherClient.DataGathers().UpdateStatus(ctx, &dg, metav1.UpdateOptions{})
	return err
}

// copyDataGatherStatusToOperatorStatus gets the "cluster" "insightsoperator.operator.openshift.io" resource
// and updates its status with values from the provided "datagather.insights.openshift.io" resource.
func (c *Controller) copyDataGatherStatusToOperatorStatus(ctx context.Context,
	dataGather *insightsv1alpha1.DataGather) (*v1.InsightsOperator, error) {
	operator, err := c.insightsOperatorCLI.Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	statusToUpdate := status.DataGatherStatusToOperatorStatus(dataGather)
	operator.Status = statusToUpdate

	_, err = c.insightsOperatorCLI.UpdateStatus(ctx, operator, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return operator, nil
}

// updateOperatorStatusCR gets the 'cluster' insightsoperators.operator.openshift.io resource and updates its status with the last
// gathering details.
func (c *Controller) updateOperatorStatusCR(ctx context.Context, allFunctionReports map[string]gather.GathererFunctionReport,
	gatherTime metav1.Time) error {
	insightsOperatorCR, err := c.insightsOperatorCLI.Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}

	updatedOperatorCR := insightsOperatorCR.DeepCopy()
	updatedOperatorCR.Status.GatherStatus = v1.GatherStatus{
		LastGatherTime: gatherTime,
		LastGatherDuration: metav1.Duration{
			Duration: time.Since(gatherTime.Time),
		},
	}

	for k := range allFunctionReports {
		fr := allFunctionReports[k]
		// duration = 0 means the gatherer didn't run
		if fr.Duration == 0 {
			continue
		}

		gs := status.CreateOperatorGathererStatus(&fr)
		updatedOperatorCR.Status.GatherStatus.Gatherers = append(updatedOperatorCR.Status.GatherStatus.Gatherers, gs)
	}

	_, err = c.insightsOperatorCLI.UpdateStatus(ctx, updatedOperatorCR, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// isGatheringDisabled checks and returns whether the data gathering
// is disabled or not. There are two options to disable it:
// - removing the corresponding token from pull-secret (the first and original option)
// - configure it in the "insightsdatagather.config.openshift.io" CR
func (c *Controller) isGatheringDisabled() bool {
	// old way of disabling data gathering by removing
	// the "cloud.openshift.com" token from the pull-secret
	if !c.secretConfigurator.Config().Report {
		return true
	}

	// disabled in the `insightsdatagather.config.openshift.io` API
	if c.apiConfigurator != nil {
		return c.apiConfigurator.GatherDisabled()
	}

	return false
}

// getInsightsImage reads "insights-operator" deployment and gets the image from the first container
func (c *Controller) getInsightsImage(ctx context.Context) (string, error) {
	insightsDeployment, err := c.kubeClient.AppsV1().Deployments(insightsNamespace).
		Get(ctx, "insights-operator", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	containers := insightsDeployment.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return "", fmt.Errorf("no container defined in the deployment")
	}
	return containers[0].Image, nil
}

// PeriodicPrune runs periodically and deletes jobs (including the related pods)
// and "datagather.insights.openshift.io" resources older than 24 hours
func (c *Controller) PeriodicPrune(ctx context.Context) {
	klog.Infof("Pruning old jobs every %s", c.pruneInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.pruneInterval):
			klog.Info("Pruning the jobs and datagather resources")
			// prune old jobs
			jobs, err := c.kubeClient.BatchV1().Jobs(insightsNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				klog.Error(err)
			}
			for i := range jobs.Items {
				job := jobs.Items[i]
				// TODO the time duration should be configurable
				if time.Since(job.CreationTimestamp.Time) > 24*time.Hour {
					err = c.kubeClient.BatchV1().Jobs(insightsNamespace).Delete(ctx, job.Name, metav1.DeleteOptions{
						PropagationPolicy: &deletePropagationBackground,
					})
					if err != nil {
						klog.Errorf("Failed to delete job %s: %v", job.Name, err)
						continue
					}
					klog.Infof("Job %s successfully removed", job.Name)
				}
			}
			// prune old DataGather custom resources
			dataGatherCRs, err := c.dataGatherClient.DataGathers().List(ctx, metav1.ListOptions{})
			if err != nil {
				klog.Error(err)
			}
			for i := range dataGatherCRs.Items {
				dataGatherCR := dataGatherCRs.Items[i]
				if time.Since(dataGatherCR.CreationTimestamp.Time) > 24*time.Hour {
					err = c.dataGatherClient.DataGathers().Delete(ctx, dataGatherCR.Name, metav1.DeleteOptions{})
					if err != nil {
						klog.Errorf("Failed to delete DataGather custom resources %s: %v", dataGatherCR.Name, err)
						continue
					}
					klog.Infof("DataGather %s resource successfully removed", dataGatherCR.Name)
				}
			}
		}
	}
}

// createNewDataGatherCR creates a new "datagather.insights.openshift.io" custom resource
// with generate name prefix "periodic-gathering-". Returns the name of the newly created
// resource
func (c *Controller) createNewDataGatherCR(ctx context.Context, disabledGatherers []string,
	dataPolicy insightsv1alpha1.DataPolicy) (*insightsv1alpha1.DataGather, error) {
	dataGatherCR := insightsv1alpha1.DataGather{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "periodic-gathering-",
		},
		Spec: insightsv1alpha1.DataGatherSpec{
			DataPolicy: dataPolicy,
		},
	}
	for _, g := range disabledGatherers {
		dataGatherCR.Spec.Gatherers = append(dataGatherCR.Spec.Gatherers, insightsv1alpha1.GathererConfig{
			Name:  g,
			State: insightsv1alpha1.Disabled,
		})
	}
	dataGather, err := c.dataGatherClient.DataGathers().Create(ctx, &dataGatherCR, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	klog.Infof("Created a new %s DataGather custom resource", dataGather.Name)
	return dataGather, nil
}

// createDataGatherAttributeValues reads the current "insightsdatagather.config.openshift.io" configuration
// and checks custom period gatherers and returns list of disabled gatherers based on this two values
// and also data policy set in the "insightsdatagather.config.openshift.io"
func (c *Controller) createDataGatherAttributeValues() ([]string, insightsv1alpha1.DataPolicy) {
	gatherConfig := c.apiConfigurator.GatherConfig()

	var dp insightsv1alpha1.DataPolicy
	switch gatherConfig.DataPolicy {
	case "":
		dp = insightsv1alpha1.NoPolicy
	case configv1alpha1.NoPolicy:
		dp = insightsv1alpha1.NoPolicy
	case configv1alpha1.ObfuscateNetworking:
		dp = insightsv1alpha1.ObfuscateNetworking
	}

	disabledGatherers := gatherConfig.DisabledGatherers
	for _, gatherer := range c.gatherers {
		if g, ok := gatherer.(gatherers.CustomPeriodGatherer); ok {
			if !g.ShouldBeProcessedNow() {
				disabledGatherers = append(disabledGatherers, g.GetName())
			} else {
				g.UpdateLastProcessingTime()
			}
		}
	}
	return disabledGatherers, dp
}

// wasDataUploaded reads status conditions of the provided "dataGather" "datagather.insights.openshift.io"
// custom resource and checks whether the data was successfully uploaded or not and updates status accordingly
func (c *Controller) wasDataUploaded(dataGather *insightsv1alpha1.DataGather) bool {
	dataUploadedCon := status.GetConditionByStatus(dataGather, status.DataUploaded)
	statusSummary := controllerstatus.Summary{
		Operation: controllerstatus.Uploading,
		Healthy:   true,
	}
	if dataUploadedCon == nil {
		statusSummary.Healthy = false
		statusSummary.Count = 5
		statusSummary.Reason = "DataUploadedConditionNotAvailable"
		statusSummary.Message = fmt.Sprintf("did not find any %q condition in the %s dataGather resource",
			status.DataUploaded, dataGather.Name)
	} else if dataUploadedCon.Status == metav1.ConditionFalse {
		statusSummary.Healthy = false
		statusSummary.Count = 5
		statusSummary.Reason = dataUploadedCon.Reason
		statusSummary.Message = dataUploadedCon.Message
	}

	c.statuses["insightsuploader"].UpdateStatus(statusSummary)
	return statusSummary.Healthy
}

func (c *Controller) wasDataProcessed(dataGather *insightsv1alpha1.DataGather) bool {
	dataProcessedCon := status.GetConditionByStatus(dataGather, status.DataProcessed)
	if dataProcessedCon == nil {
		return false
	}
	return dataProcessedCon.Status == metav1.ConditionTrue
}

// updateMetrics reads the HTTP status code from the reason of the "DataUploaded" condition
// from the provided "datagather" resource and increments
// the "insightsclient_request_send_total" Prometheus metric accordingly.
func updateMetrics(dataGather *insightsv1alpha1.DataGather) {
	dataUploadedCon := status.GetConditionByStatus(dataGather, status.DataUploaded)
	var statusCode int
	if dataUploadedCon == nil {
		statusCode = 0
	} else {
		statusCodeStr, _ := strings.CutPrefix(dataUploadedCon.Reason, "HttpStatus")
		var err error
		statusCode, err = strconv.Atoi(statusCodeStr)
		if err != nil {
			klog.Error("failed to update the Prometheus metrics: %v", err)
			return
		}
	}
	insights.IncrementCounterRequestSend(statusCode)
}

func mapToArray(m map[string]gather.GathererFunctionReport) []gather.GathererFunctionReport {
	a := make([]gather.GathererFunctionReport, 0, len(m))
	for _, v := range m {
		a = append(a, v)
	}
	return a
}
