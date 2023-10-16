package configobserver

import (
	"context"
	"sync"

	"github.com/openshift/insights-operator/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const insightsNamespaceName = "openshift-insights"

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
	config             *config.InsightsConfiguration
	listeners          map[chan struct{}]struct{}
	usingInformer      bool
	kubeClient         kubernetes.Interface
}

func NewConfigAggregator(ctrl Configurator, configMapInf ConfigMapInformer) Interface {
	confAggreg := &ConfigAggregator{
		legacyConfigurator: ctrl,
		configMapInformer:  configMapInf,
		listeners:          make(map[chan struct{}]struct{}),
		usingInformer:      true,
	}
	confAggreg.mergeUsingInformer()
	return confAggreg
}

// NewStaticConfigAggregator is a constructor used mainly for the techpreview configuration reading.
// There is no reason to create and start any informer in the techpreview when data gathering runs as a job.
// It is sufficient to read the config once when the job is created and/or starting.
func NewStaticConfigAggregator(ctrl Configurator, cli kubernetes.Interface) Interface {
	confAggreg := &ConfigAggregator{
		legacyConfigurator: ctrl,
		configMapInformer:  nil,
		kubeClient:         cli,
	}

	confAggreg.mergeStatically()
	return confAggreg
}

// mergeUsingInformer merges config values for the legacy "support" secret configuration and
// from the new configmap informer. The "insights-config" configmap always takes
// precedence if it exists and is not empty.
func (c *ConfigAggregator) mergeUsingInformer() {
	c.lock.Lock()
	defer c.lock.Unlock()
	newConf := c.configMapInformer.Config()
	conf := c.legacyConfigToInsightsConfiguration()

	if newConf == nil {
		c.config = conf
		return
	}

	c.merge(conf, newConf)
}

// mergeStatically merges config values for the legacy "support" secret configuration and
// from the "insights-config" configmap by getting and reading the confimap directly without
// using an informer.
func (c *ConfigAggregator) mergeStatically() {
	c.lock.Lock()
	defer c.lock.Unlock()
	conf := c.legacyConfigToInsightsConfiguration()
	c.config = conf
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cm, err := c.kubeClient.CoreV1().ConfigMaps(insightsNamespaceName).Get(ctx, insightsConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return
		}
		klog.Error(err)
	}

	cmConf, err := readConfigAndDecode(cm)
	if err != nil {
		klog.Error("Failed to read configmap configuration: %v", err)
		return
	}

	c.merge(conf, cmConf)
}

func (c *ConfigAggregator) merge(defaultCfg, newCfg *config.InsightsConfiguration) {
	// read config map values and merge
	if newCfg.DataReporting.Interval != 0 {
		defaultCfg.DataReporting.Interval = newCfg.DataReporting.Interval
	}

	if newCfg.DataReporting.UploadEndpoint != "" {
		defaultCfg.DataReporting.UploadEndpoint = newCfg.DataReporting.UploadEndpoint
	}

	if newCfg.DataReporting.DownloadEndpoint != "" {
		defaultCfg.DataReporting.DownloadEndpoint = newCfg.DataReporting.DownloadEndpoint
	}

	if newCfg.DataReporting.DownloadEndpointTechPreview != "" {
		defaultCfg.DataReporting.DownloadEndpointTechPreview = newCfg.DataReporting.DownloadEndpointTechPreview
	}

	if newCfg.DataReporting.ProcessingStatusEndpoint != "" {
		defaultCfg.DataReporting.ProcessingStatusEndpoint = newCfg.DataReporting.ProcessingStatusEndpoint
	}

	if newCfg.DataReporting.ConditionalGathererEndpoint != "" {
		defaultCfg.DataReporting.ConditionalGathererEndpoint = newCfg.DataReporting.ConditionalGathererEndpoint
	}

	if newCfg.DataReporting.StoragePath != "" {
		defaultCfg.DataReporting.StoragePath = newCfg.DataReporting.StoragePath
	}
	c.config = defaultCfg
}

func (c *ConfigAggregator) Config() *config.InsightsConfiguration {
	if c.usingInformer {
		c.mergeUsingInformer()
	} else {
		c.mergeStatically()
	}
	return c.config
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

func (c *ConfigAggregator) legacyConfigToInsightsConfiguration() *config.InsightsConfiguration {
	legacyConfig := c.legacyConfigurator.Config()
	return &config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			Interval:                    legacyConfig.Interval,
			UploadEndpoint:              legacyConfig.Endpoint,
			DownloadEndpoint:            legacyConfig.ReportEndpoint,
			ConditionalGathererEndpoint: legacyConfig.ConditionalGathererEndpoint,
			ProcessingStatusEndpoint:    legacyConfig.ProcessingStatusEndpoint,
			DownloadEndpointTechPreview: legacyConfig.ReportEndpointTechPreview,
			// This can't be overridden by the config map - it's not merged in the merge function.
			// The value is based on the presence of the token in the pull-secret and the config map
			// doesn't know anything about secrets
			Enabled:            legacyConfig.Report,
			StoragePath:        legacyConfig.StoragePath,
			ReportPullingDelay: legacyConfig.ReportPullingDelay,
		},
	}
}
