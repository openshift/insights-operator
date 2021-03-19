package periodic

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/klog"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/record"
)

type Configurator interface {
	Config() *config.Controller
	ConfigChanged() (<-chan struct{}, func())
}

type Controller struct {
	config    Configurator
	recorder  record.FlushInterface
	gatherers map[string]gather.Interface
	status    map[string]*controllerstatus.Simple
	queue     workqueue.RateLimitingInterface
}

func New(config Configurator, recorder record.FlushInterface, gatherers map[string]gather.Interface) *Controller {
	status := make(map[string]*controllerstatus.Simple)
	for k := range gatherers {
		status[k] = &controllerstatus.Simple{Name: fmt.Sprintf("periodic-%s", k)}
	}
	c := &Controller{
		config:    config,
		recorder:  recorder,
		gatherers: gatherers,
		status:    status,

		// TODO: tune rate limiter here for non-aggressive action
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "gatherer"),
	}
	return c
}

func (c *Controller) Sources() []controllerstatus.Interface {
	keys := make([]string, 0, len(c.status))
	for key := range c.status {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sources := make([]controllerstatus.Interface, 0, len(keys))
	for _, key := range keys {
		sources = append(sources, c.status[key])
	}
	return sources
}

// sync gathers data from the cluster periodically
func (c *Controller) sync(name string) error {
	gatherer, ok := c.gatherers[name]
	if !ok {
		klog.V(2).Infof("No such gatherer %s", name)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Config().Interval/2)
	defer cancel()
	defer func() {
		if err := c.recorder.Flush(ctx); err != nil {
			klog.Errorf("Unable to flush recorder: %v", err)
		}
	}()
	klog.V(4).Infof("Running %s", name)
	return gatherer.Gather(ctx, c.recorder)
}

func (c *Controller) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	defer klog.Info("Shutting down")

	// start watching for version changes
	go wait.Until(func() { c.periodicTrigger(stopCh) }, time.Second, stopCh)

	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	// seed the queue
	c.Gather()

	<-stopCh
}

func (c *Controller) Gather() {
	for name := range c.gatherers {
		c.queue.Add(name)
	}
}

func (c *Controller) periodicTrigger(stopCh <-chan struct{}) {
	configCh, closeFn := c.config.ConfigChanged()
	defer closeFn()

	interval := c.config.Config().Interval
	expireCh := time.After(wait.Jitter(interval, 0.5))
	klog.Infof("Gathering cluster info every %s", interval)
	for {
		select {
		case <-stopCh:
			return

		case <-configCh:
			newInterval := c.config.Config().Interval
			if newInterval == interval {
				continue
			}
			interval = newInterval
			klog.Infof("Gathering cluster info every %s", interval)
		case <-expireCh:
		}

		for name := range c.gatherers {
			c.queue.AddAfter(name, wait.Jitter(interval/4, 2))
		}
		expireCh = time.After(wait.Jitter(interval, 0.5))
	}
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(dsKey)
	name := dsKey.(string)
	start := time.Now()
	err := c.sync(name)
	if err == nil {
		klog.V(4).Infof("Periodic gather %s completed in %s", name, time.Now().Sub(start).Truncate(time.Millisecond))
		c.queue.Forget(dsKey)
		c.status[name].UpdateStatus(controllerstatus.Summary{Healthy: true})
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed after %s with: %v", dsKey, time.Now().Sub(start).Truncate(time.Millisecond), err))
	c.queue.AddRateLimited(dsKey)
	summary := controllerstatus.Summary{Operation: controllerstatus.GatheringReport, Reason: "PeriodicGatherFailed", Message: fmt.Sprintf("Source %s could not be retrieved: %v", name, err)}
	c.status[name].UpdateStatus(summary)

	return true
}
