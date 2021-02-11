package periodic

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/klog/v2"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/recorder"
)

type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

type Controller struct {
	configurator Configurator
	recorder     recorder.FlushInterface
	gatherers    map[string]gather.Interface
	statuses     map[string]*controllerstatus.Simple
}

func New(configurator Configurator, recorder recorder.FlushInterface, gatherers map[string]gather.Interface) *Controller {
	statuses := make(map[string]*controllerstatus.Simple)
	for k := range gatherers {
		statuses[k] = &controllerstatus.Simple{Name: fmt.Sprintf("periodic-%s", k)}
	}
	c := &Controller{
		configurator: configurator,
		recorder:     recorder,
		gatherers:    gatherers,
		statuses:     statuses,
	}
	return c
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

// Runs the gatherers one after the other.
// Currently their is only 1 gatherer (clusterconfig) and no new gatherer is on the horizon.
// Running the gatherers in parallel should be a future improvement when a new gatherer is introduced.
func (c *Controller) Gather() {
	interval := c.configurator.Config().Interval
	threshold := status.GatherFailuresCountThreshold
	duration := interval / (time.Duration(threshold) * 2)
	// IMPORTANT: We NEED to run retry $threshold times or we will never set status to degraded.
	backoff := wait.Backoff{
		Duration: duration,
		Factor:   1.35,
		Jitter:   0,
		Steps:    threshold,
		Cap:      interval,
	}
	for name := range c.gatherers {
		_ = wait.ExponentialBackoff(backoff, func() (bool, error) {
			start := time.Now()
			err := c.runGatherer(name)
			if err == nil {
				klog.V(3).Infof("Periodic gather %s completed in %s", name, time.Since(start).Truncate(time.Millisecond))
				c.statuses[name].UpdateStatus(controllerstatus.Summary{Healthy: true})
				return true, nil
			}
			utilruntime.HandleError(fmt.Errorf("%v failed after %s with: %v", name, time.Since(start).Truncate(time.Millisecond), err))
			c.statuses[name].UpdateStatus(controllerstatus.Summary{Operation: controllerstatus.GatheringReport, Reason: "PeriodicGatherFailed", Message: fmt.Sprintf("Source %s could not be retrieved: %v", name, err)})
			return false, nil
		})
	}
}

// Does the prep for running a gatherer then calls gatherer.Gather. (getting the context, cleaning the recorder)
func (c *Controller) runGatherer(name string) error {
	gatherer, ok := c.gatherers[name]
	if !ok {
		klog.V(2).Infof("No such gatherer %s", name)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.configurator.Config().Interval/2)
	defer cancel()
	defer func() {
		if err := c.recorder.Flush(ctx); err != nil {
			klog.Errorf("Unable to flush recorder: %v", err)
		}
	}()
	klog.V(4).Infof("Running %s", name)
	return gatherer.Gather(ctx, c.configurator.Config().Gather, c.recorder)
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
