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

type Controller struct {
	interval time.Duration

	configurator config.Configurator
	recorder     record.FlushInterface
	gatherers    map[string]gather.Interface
	status       map[string]*controllerstatus.Simple
	queue        workqueue.RateLimitingInterface
}

func New(configurator config.Configurator, recorder record.FlushInterface, gatherers map[string]gather.Interface) *Controller {
	status := make(map[string]*controllerstatus.Simple)
	for k := range gatherers {
		status[k] = &controllerstatus.Simple{Name: fmt.Sprintf("periodic-%s", k)}
	}
	c := &Controller{
		configurator: configurator,
		recorder:     recorder,
		gatherers:    gatherers,
		status:       status,

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
	ctx, cancel := context.WithTimeout(context.Background(), c.interval/2)
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

	c.updateConfig()
	configCh := c.watchConfig(stopCh)

	klog.Infof("Gathering cluster info every %s", c.interval)
	defer klog.Info("Shutting down")

	// start watching for version changes
	go wait.Until(func() { c.periodicTrigger(configCh, stopCh) }, time.Second, stopCh)

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

func (c *Controller) periodicTrigger(configCh, stopCh <-chan struct{}) {
	for {
		select {
		case <-configCh:
			return
		case <-stopCh:
			return
		case <-time.After(wait.Jitter(c.interval, 0.5)):
			for name := range c.gatherers {
				c.queue.AddAfter(name, wait.Jitter(c.interval/4, 2))
			}
		}
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
	c.status[name].UpdateStatus(controllerstatus.Summary{Reason: "PeriodicGatherFailed", Message: fmt.Sprintf("Source %s could not be retrieved: %v", name, err)})

	return true
}

func (c *Controller) updateConfig() bool {
	cfg := c.configurator.Config()
	if c.interval != cfg.Interval {
		c.interval = cfg.Interval
		klog.V(4).Infof("Gathering interval set to %s", cfg.Interval)
		return true
	} else {
		return false
	}
}

// Runs loop for checking on configuration change to check for periodic-related changes.
// Returns a channel that it writes to, when the config change influenced the controller.
func (c *Controller) watchConfig(stopCh <-chan struct{}) chan struct{} {
	configCh := make(chan struct{}, 1)
	globalConfigCh, cancelFn := c.configurator.ConfigChanged()

	watchChanges := func() {
		defer cancelFn()
		for {
			select {
			case <-globalConfigCh:
				if c.updateConfig() {
					configCh <- struct{}{}
				}
			case <-stopCh:
				return
			}
		}
	}
	go watchChanges()
	return configCh
}
