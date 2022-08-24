package periodic

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	v1 "github.com/openshift/api/operator/v1"
	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/recorder"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DataGatheredCondition = "DataGathered"
	// NoDataGathered is a reason when there is no data gathered - e.g the resource is not in a cluster
	NoDataGatheredReason = "NoData"
	// Error is a reason when there is some error and no data gathered
	GatherErrorReason = "Error"
	// Panic is a reason when there is some error and no data gathered
	GatherPanicReason = "Panic"
	// GatheredOK is a reason when data is gathered as expected
	GatherOKReason = "GatheredOK"
	// GatherWithError is a reason when data is gathered partially or with another error message
	GatherWithErrorReason = "GatheredWithError"
)

// Controller periodically runs gatherers, records their results to the recorder
// and flushes the recorder to create archives
type Controller struct {
	configurator        configobserver.Configurator
	recorder            recorder.FlushInterface
	gatherers           []gatherers.Interface
	statuses            map[string]controllerstatus.StatusController
	anonymizer          *anonymization.Anonymizer
	insightsOperatorCLI operatorv1client.InsightsOperatorInterface
}

// New creates a new instance of Controller which periodically invokes the gatherers
// and flushes the recorder to create archives.
func New(
	configurator configobserver.Configurator,
	rec recorder.FlushInterface,
	listGatherers []gatherers.Interface,
	anonymizer *anonymization.Anonymizer,
	insightsOperatorCLI operatorv1client.InsightsOperatorInterface,
) *Controller {
	statuses := make(map[string]controllerstatus.StatusController)

	for _, gatherer := range listGatherers {
		gathererName := gatherer.GetName()
		statuses[gathererName] = controllerstatus.New(fmt.Sprintf("periodic-%s", gathererName))
	}

	return &Controller{
		configurator:        configurator,
		recorder:            rec,
		gatherers:           listGatherers,
		statuses:            statuses,
		anonymizer:          anonymizer,
		insightsOperatorCLI: insightsOperatorCLI,
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
			c.Gather()
		}
	} else {
		c.Gather()
	}

	go wait.Until(func() { c.periodicTrigger(stopCh) }, time.Second, stopCh)

	<-stopCh
}

// Gather Runs the gatherers one after the other.
func (c *Controller) Gather() {
	if !c.configurator.Config().Report {
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

			ctx, cancel := context.WithTimeout(context.Background(), c.configurator.Config().Interval/2)
			defer cancel()

			klog.V(4).Infof("Running %s gatherer", gatherer.GetName())
			functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, c.recorder, c.configurator)
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
func (c *Controller) periodicTrigger(stopCh <-chan struct{}) {
	configCh, closeFn := c.configurator.ConfigChanged()
	defer closeFn()

	interval := c.configurator.Config().Interval
	klog.Infof("Gathering cluster info every %s", interval)
	for {
		select {
		case <-stopCh:
			return

		case <-configCh:
			newInterval := c.configurator.Config().Interval
			if newInterval == interval {
				continue
			}
			interval = newInterval
			klog.Infof("Gathering cluster info every %s", interval)

		case <-time.After(interval):
			c.Gather()
		}
	}
}

func (c *Controller) updateOperatorStatusCR(allFunctionReports map[string]gather.GathererFunctionReport, gatherTime metav1.Time) error {
	insightsOperatorCR, err := c.insightsOperatorCLI.Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}

	ioCopy := insightsOperatorCR.DeepCopy()
	ioCopy.Status.GatherStatus.Gatherers = []v1.GathererStatus{}
	ioCopy.Status.GatherStatus.LastGatherTime = gatherTime
	ioCopy.Status.GatherStatus.LastGatherDuration = metav1.Duration{
		Duration: time.Since(gatherTime.Time),
	}

	for k := range allFunctionReports {
		fr := allFunctionReports[k]
		// duration = 0 means the gatherer didn't run
		if fr.Duration == 0 {
			continue
		}

		gs := createGathererStatus(&fr)
		ioCopy.Status.GatherStatus.Gatherers = append(ioCopy.Status.GatherStatus.Gatherers, gs)
	}

	_, err = c.insightsOperatorCLI.UpdateStatus(context.Background(), ioCopy, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
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
		con.Reason = GatherOKReason
		con.Message = fmt.Sprintf("Created %d records in the archive.", gfr.RecordsCount)

		if len(gfr.Errors) > 0 {
			con.Reason = GatherWithErrorReason
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

func mapToArray(m map[string]gather.GathererFunctionReport) []gather.GathererFunctionReport {
	a := make([]gather.GathererFunctionReport, 0, len(m))
	for _, v := range m {
		a = append(a, v)
	}
	return a
}
