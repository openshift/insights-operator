package periodic

import (
	"context"
	"fmt"
	"sort"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/recorder"
)

// Controller periodically runs gatherers, records their results to the recorder
// and flushes the recorder to create archives
type Controller struct {
	configurator configobserver.Configurator
	recorder     recorder.FlushInterface
	gatherers    []gatherers.Interface
	statuses     map[string]*controllerstatus.Simple
	anonymizer   *anonymization.Anonymizer
}

// New creates a new instance of Controller which periodically invokes the gatherers
// and flushes the recorder to create archives.
func New(
	configurator configobserver.Configurator,
	rec recorder.FlushInterface,
	listGatherers []gatherers.Interface,
	anonymizer *anonymization.Anonymizer,
) *Controller {
	statuses := make(map[string]*controllerstatus.Simple)

	for _, gatherer := range listGatherers {
		gathererName := gatherer.GetName()
		statuses[gathererName] = &controllerstatus.Simple{Name: fmt.Sprintf("periodic-%s", gathererName)}
	}

	return &Controller{
		configurator: configurator,
		recorder:     rec,
		gatherers:    listGatherers,
		statuses:     statuses,
		anonymizer:   anonymizer,
	}
}

func (c *Controller) Sources() []controllerstatus.Interface {
	keys := make([]string, 0, len(c.statuses))
	for key := range c.statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sources := make([]controllerstatus.Interface, 0, len(keys))
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

	for _, gatherer := range gatherersToProcess {
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
			continue
		}

		utilruntime.HandleError(fmt.Errorf("%v failed after %s with: %v", name, time.Since(start).Truncate(time.Millisecond), err))
		/* 			c.statuses[name].UpdateStatus(
		controllerstatus.Summary{
			Operation: controllerstatus.GatheringReport,
			Reason:    "PeriodicGatherFailed",
			Message:   fmt.Sprintf("Source %s could not be retrieved: %v", name, err),
		}) */
	}

	err := gather.RecordArchiveMetadata(mapToArray(allFunctionReports), c.recorder, c.anonymizer)
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

func mapToArray(m map[string]gather.GathererFunctionReport) []gather.GathererFunctionReport {
	a := make([]gather.GathererFunctionReport, 0, len(m))
	for _, v := range m {
		a = append(a, v)
	}
	return a
}
