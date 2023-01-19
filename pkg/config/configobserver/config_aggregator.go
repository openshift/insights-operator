package configobserver

import (
	"context"
	"sync"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"k8s.io/klog/v2"
)

type Interface interface {
	Config() *config.InsightsConfiguration
	ConfigChanged() (<-chan struct{}, func())
	Listen(ctx context.Context)
}

// ConfigAggregator is an auxiliary structure that should obviate the need for the use of
// legacy secret configurator and the new config map informer
type ConfigAggregator struct {
	lock               sync.Mutex
	legacyConfigurator Configurator
	configMapInformer  ConfigMapInformer
	configAggregated   *config.InsightsConfiguration
	listeners          map[chan struct{}]struct{}
}

func NewConfigAggregator(ctrl Configurator, configMapInf ConfigMapInformer) Interface {
	confAggreg := &ConfigAggregator{
		legacyConfigurator: ctrl,
		configMapInformer:  configMapInf,
		listeners:          make(map[chan struct{}]struct{}),
	}
	confAggreg.merge()
	return confAggreg
}

// merge merges config values for the legacy "support" secret configuration and
// from the new configmap informer. The "insights-config" configmap always takes
// precedence if it exists and is not empty.
func (c *ConfigAggregator) merge() {
	legacyConfig := c.legacyConfigurator.Config()
	newConf := c.configMapInformer.Config()
	conf := &config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			Interval:         legacyConfig.Interval,
			UploadEndpoint:   legacyConfig.Endpoint,
			DownloadEndpoint: legacyConfig.ReportEndpoint,
			// This can't be overridden by the config map - it's not merged below.
			// The value is based on the presence of the token in the pull-secret and the config map
			// doesn't know anything about secrets
			Enabled: legacyConfig.Report,
		},
	}

	if newConf == nil {
		c.configAggregated = conf
		klog.Infof("Merged config is: %v", c.configAggregated)
		return
	}

	// read config map values and merge
	if newConf.DataReporting.Interval != 0*time.Minute {
		conf.DataReporting.Interval = newConf.DataReporting.Interval
	}

	if newConf.DataReporting.UploadEndpoint != "" {
		conf.DataReporting.UploadEndpoint = newConf.DataReporting.UploadEndpoint
	}

	if newConf.DataReporting.DownloadEndpoint != "" {
		conf.DataReporting.DownloadEndpoint = newConf.DataReporting.DownloadEndpoint
	}

	c.configAggregated = conf
	klog.Infof("Merged config is: %v", c.configAggregated)
}

func (c *ConfigAggregator) Config() *config.InsightsConfiguration {
	c.merge()
	return c.configAggregated
}

// Listen listens to the legacy Secret configurator/observer as well as the
// new config map informer. When any configuration change is observed then all the listeners
// are notified.
func (c *ConfigAggregator) Listen(ctx context.Context) {
	legacyCh, legacyCloseFn := c.legacyConfigurator.ConfigChanged()
	cmCh, cmICloseFn := c.configMapInformer.ConfigChanged()
	defer func() {
		legacyCloseFn()
		cmICloseFn()
	}()

	for {
		select {
		case <-legacyCh:
			c.notifyListeners()
		case <-cmCh:
			c.notifyListeners()
		case <-ctx.Done():
			return
		}
	}
}

func (c *ConfigAggregator) notifyListeners() {
	for ch := range c.listeners {
		ch <- struct{}{}
	}
}

func (c *ConfigAggregator) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
	c.lock.Lock()
	defer c.lock.Unlock()
	ch := make(chan struct{}, 1)
	c.listeners[ch] = struct{}{}
	return ch, func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		close(ch)
		delete(c.listeners, ch)
	}
}
