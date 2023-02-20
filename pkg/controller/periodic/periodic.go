package periodic

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	v1 "github.com/openshift/api/operator/v1"
	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/insights/insightsreport"
	"github.com/openshift/insights-operator/pkg/recorder"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DataGatheredCondition = "DataGathered"
	// NoDataGathered is a reason when there is no data gathered - e.g the resource is not in a cluster
	NoDataGatheredReason = "NoData"
	// Error is a reason when there is some error and no data gathered
	GatherErrorReason = "GatherError"
	// Panic is a reason when there is some error and no data gathered
	GatherPanicReason = "GatherPanic"
	// GatheredOK is a reason when data is gathered as expected
	GatheredOKReason = "GatheredOK"
	// GatheredWithError is a reason when data is gathered partially or with another error message
	GatheredWithErrorReason = "GatheredWithError"
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
	apiConfigurator     configobserver.APIConfigObserver
	recorder            recorder.FlushInterface
	gatherers           []gatherers.Interface
	statuses            map[string]controllerstatus.StatusController
	anonymizer          *anonymization.Anonymizer
	insightsOperatorCLI operatorv1client.InsightsOperatorInterface
	kubeClient          *kubernetes.Clientset
	reportRetriever     *insightsreport.Controller
	image               string
}

func NewWithTechPreview(
	reportRetriever *insightsreport.Controller,
	secretConfigurator configobserver.Configurator,
	apiConfigurator configobserver.APIConfigObserver,
	kubeClient *kubernetes.Clientset,
) *Controller {
	statuses := make(map[string]controllerstatus.StatusController)
	return &Controller{
		reportRetriever:    reportRetriever,
		secretConfigurator: secretConfigurator,
		apiConfigurator:    apiConfigurator,
		statuses:           statuses,
		kubeClient:         kubeClient,
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
	apiConfigurator configobserver.APIConfigObserver,
	kubeClient *kubernetes.Clientset,
) *Controller {
	statuses := make(map[string]controllerstatus.StatusController)

	for _, gatherer := range listGatherers {
		gathererName := gatherer.GetName()
		statuses[gathererName] = controllerstatus.New(fmt.Sprintf("periodic-%s", gathererName))
	}

	return &Controller{
		secretConfigurator:  secretConfigurator,
		apiConfigurator:     apiConfigurator,
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

func (c *Controller) Run(stopCh <-chan struct{}, initialDelay time.Duration, techPreview bool) {
	defer utilruntime.HandleCrash()
	defer klog.Info("Shutting down")

	// Runs a gather after startup
	if initialDelay > 0 {
		select {
		case <-stopCh:
			return
		case <-time.After(initialDelay):
			if techPreview {
				c.GatherJob()
			} else {
				c.Gather()
			}
		}
	} else {
		if techPreview {
			c.GatherJob()
		} else {
			c.Gather()
		}
	}

	go wait.Until(func() { c.periodicTrigger(stopCh, techPreview) }, time.Second, stopCh)

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

	allFunctionReports := make(map[string]gather.GathererFunctionReport)
	gatherTime := metav1.Now()
	for _, gatherer := range gatherersToProcess {
		func() {
			name := gatherer.GetName()
			start := time.Now()

			ctx, cancel := context.WithTimeout(context.Background(), c.secretConfigurator.Config().Interval/2)
			defer cancel()

			klog.V(4).Infof("Running %s gatherer", gatherer.GetName())
			var functionReports []gather.GathererFunctionReport
			var err error
			if c.apiConfigurator != nil {
				functionReports, err = gather.CollectAndRecordGatherer(ctx, gatherer, c.recorder, c.apiConfigurator.GatherConfig())
			} else {
				functionReports, err = gather.CollectAndRecordGatherer(ctx, gatherer, c.recorder, nil)
			}
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
	err := c.updateOperatorStatusCR(allFunctionReports, gatherTime)
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
func (c *Controller) periodicTrigger(stopCh <-chan struct{}, techPreview bool) {
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
			if techPreview {
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
	if c.image == "" {
		image, err := c.getInsightsImage()
		if err != nil {
			klog.Errorf("Can't get operator image. Gathering will not run: %v", err)
			return
		}
		c.image = image
	}

	gj := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "periodic-gathering-",
			Namespace:    insightsNamespace,
		},
		Spec: batchv1.JobSpec{
			// backoff limit is 0 - we dont' want to restart the gathering immediately in case of failure
			BackoffLimit: new(int32),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "operator",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &trueB,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "archives-path",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: serviceCABundle,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: serviceCABundle,
									},
									Optional: &trueB,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "insights-gathering",
							Image: c.image,
							Args:  []string{"gather-and-upload", "-v=4", "--config=/etc/insights-operator/server.yaml"},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: falseB,
								Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "archives-path",
									MountPath: c.secretConfigurator.Config().StoragePath,
								},
								{
									Name:      serviceCABundle,
									MountPath: serviceCABundlePath,
								},
							},
						},
					},
				},
			},
		},
	}

	klog.Infof("Creating gathering job %v", gj.Name)
	gj, err := c.kubeClient.BatchV1().Jobs(insightsNamespace).Create(context.Background(), gj, metav1.CreateOptions{})
	if err != nil {
		klog.Error(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.secretConfigurator.Config().Interval*4)
	defer cancel()
	err = c.waitForJobCompletion(ctx, gj)
	if err != nil {
		if err == context.DeadlineExceeded {
			klog.Errorf("Failed to read job status: %v", err)
			return
		}
		klog.Error(err)
	}
	klog.Infof("Job completed %s", gj.Name)
	// TODO read the status of the CR and copy to insightsoperator CR
	c.reportRetriever.RetrieveReport()
}

// updateOperatorStatusCR gets the 'cluster' insightsoperators.operator.openshift.io resource and updates its status with the last
// gathering details.
func (c *Controller) updateOperatorStatusCR(allFunctionReports map[string]gather.GathererFunctionReport, gatherTime metav1.Time) error {
	insightsOperatorCR, err := c.insightsOperatorCLI.Get(context.Background(), "cluster", metav1.GetOptions{})
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

		gs := createGathererStatus(&fr)
		updatedOperatorCR.Status.GatherStatus.Gatherers = append(updatedOperatorCR.Status.GatherStatus.Gatherers, gs)
	}

	_, err = c.insightsOperatorCLI.UpdateStatus(context.Background(), updatedOperatorCR, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

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
func (c *Controller) getInsightsImage() (string, error) {
	insightsDeployment, err := c.kubeClient.AppsV1().Deployments(insightsNamespace).
		Get(context.Background(), "insights-operator", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	containers := insightsDeployment.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return "", fmt.Errorf("no container defined in the deployment")
	}
	return containers[0].Image, nil
}

func (c *Controller) waitForJobCompletion(ctx context.Context, job *batchv1.Job) error {
	return wait.PollUntil(20*time.Second, func() (done bool, err error) {
		j, err := c.kubeClient.BatchV1().Jobs(insightsNamespace).Get(ctx, job.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return false, err
		}
		if j.Status.Succeeded > 0 {
			return true, nil
		}
		if j.Status.Failed > 0 {
			return true, fmt.Errorf("job %s failed", job.Name)
		}
		// TODO check job conditions ?
		return false, nil
	}, ctx.Done())
}

func createGathererStatus(gfr *gather.GathererFunctionReport) v1.GathererStatus {
	gs := v1.GathererStatus{
		Name: gfr.FuncName,
		LastGatherDuration: metav1.Duration{
			// v.Duration is in milliseconds and we need nanoseconds
			Duration: time.Duration(gfr.Duration * 1000000),
		},
	}
	con := metav1.Condition{
		Type:               DataGatheredCondition,
		LastTransitionTime: metav1.Now(),
		Status:             metav1.ConditionFalse,
		Reason:             NoDataGatheredReason,
	}

	if gfr.Panic != nil {
		con.Reason = GatherPanicReason
		con.Message = gfr.Panic.(string)
	}

	if gfr.RecordsCount > 0 {
		con.Status = metav1.ConditionTrue
		con.Reason = GatheredOKReason
		con.Message = fmt.Sprintf("Created %d records in the archive.", gfr.RecordsCount)

		if len(gfr.Errors) > 0 {
			con.Reason = GatheredWithErrorReason
			con.Message = fmt.Sprintf("%s Error: %s", con.Message, strings.Join(gfr.Errors, ","))
		}

		gs.Conditions = append(gs.Conditions, con)
		return gs
	}

	if len(gfr.Errors) > 0 {
		con.Reason = GatherErrorReason
		con.Message = strings.Join(gfr.Errors, ",")
	}

	gs.Conditions = append(gs.Conditions, con)

	return gs
}

// PeriodicPrune runs periodically and deletes jobs (including the related pods) older
// than given time
func (c *Controller) PeriodicPrune(ctx context.Context) {
	pruneInterval := 12 * time.Hour
	klog.Infof("Pruning old jobs every %s", pruneInterval)
	for {
		select {
		case <-ctx.Done():
		case <-time.After(pruneInterval):
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
		}
	}
}

func mapToArray(m map[string]gather.GathererFunctionReport) []gather.GathererFunctionReport {
	a := make([]gather.GathererFunctionReport, 0, len(m))
	for _, v := range m {
		a = append(a, v)
	}
	return a
}
