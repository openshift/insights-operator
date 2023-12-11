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
		klog.Errorf("Failed to read configmap configuration: %v", err)
		return
	}

	c.merge(conf, cmConf)
}

// merge merges the default configuration options with the defined ones
func (c *ConfigAggregator) merge(defaultCfg, newCfg *config.InsightsConfiguration) {
	c.mergeDataReporting(defaultCfg, newCfg)
	c.mergeSCAConfig(defaultCfg, newCfg)
	c.mergeAlerting(defaultCfg, newCfg)
	c.mergeClusterTransfer(defaultCfg, newCfg)
	c.mergeProxyConfig(defaultCfg, newCfg)
	c.config = defaultCfg
}

// mergeDataReporting checks configured data reporting options and if they are not empty then
// override default data reporting configuration
func (c *ConfigAggregator) mergeDataReporting(defaultCfg, newCfg *config.InsightsConfiguration) {
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

	if len(newCfg.DataReporting.Obfuscation) > 0 {
		defaultCfg.DataReporting.Obfuscation = append(defaultCfg.DataReporting.Obfuscation, newCfg.DataReporting.Obfuscation...)
	}
}

func (c *ConfigAggregator) mergeAlerting(defaultCfg, newCfg *config.InsightsConfiguration) {
	if newCfg.Alerting.Disabled != defaultCfg.Alerting.Disabled {
		defaultCfg.Alerting.Disabled = newCfg.Alerting.Disabled
	}
}

// mergeSCAConfig checks configured SCA options and if they are not empty then
// override default SCA configuration
func (c *ConfigAggregator) mergeSCAConfig(defaultCfg, newCfg *config.InsightsConfiguration) {
	if newCfg.SCA.Interval != 0 {
		defaultCfg.SCA.Interval = newCfg.SCA.Interval
	}

	if newCfg.SCA.Endpoint != "" {
		defaultCfg.SCA.Endpoint = newCfg.SCA.Endpoint
	}

	if newCfg.SCA.Disabled != defaultCfg.SCA.Disabled {
		defaultCfg.SCA.Disabled = newCfg.SCA.Disabled
	}
}

// mergeProxyConfig checks configured proxy options and if they are not empty then
// override default connection configuration
func (c *ConfigAggregator) mergeProxyConfig(defaultCfg, newCfg *config.InsightsConfiguration) {
	if newCfg.Proxy.HTTPProxy != "" {
		defaultCfg.Proxy.HTTPProxy = newCfg.Proxy.HTTPProxy
	}

	if newCfg.Proxy.HTTPSProxy != "" {
		defaultCfg.Proxy.HTTPSProxy = newCfg.Proxy.HTTPSProxy
	}

	if newCfg.Proxy.NoProxy != "" {
		defaultCfg.Proxy.NoProxy = newCfg.Proxy.NoProxy
	}
}

// mergeClusterTransfer checks configured cluster transfer options and if they are not empty then
// override default cluster transfer configuration
func (c *ConfigAggregator) mergeClusterTransfer(defaultCfg, newCfg *config.InsightsConfiguration) {
	if newCfg.ClusterTransfer.Interval != 0 {
		defaultCfg.ClusterTransfer.Interval = newCfg.ClusterTransfer.Interval
	}

	if newCfg.ClusterTransfer.Endpoint != "" {
		defaultCfg.ClusterTransfer.Endpoint = newCfg.ClusterTransfer.Endpoint
	}
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
	var obfuscation config.Obfuscation
	if legacyConfig.EnableGlobalObfuscation {
		obfuscation = config.Obfuscation{config.Networking}
	}

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
			Obfuscation:        obfuscation,
		},
		Alerting: config.Alerting{
			Disabled: legacyConfig.DisableInsightsAlerts,
		},
		SCA: config.SCA{
			Disabled: legacyConfig.OCMConfig.SCADisabled,
			Interval: legacyConfig.OCMConfig.SCAInterval,
			Endpoint: legacyConfig.OCMConfig.SCAEndpoint,
		},
		ClusterTransfer: config.ClusterTransfer{
			Interval: legacyConfig.OCMConfig.ClusterTransferInterval,
			Endpoint: legacyConfig.OCMConfig.ClusterTransferEndpoint,
		},
		Proxy: config.Proxy{
			HTTPProxy:  legacyConfig.HTTPConfig.HTTPProxy,
			HTTPSProxy: legacyConfig.HTTPConfig.HTTPSProxy,
			NoProxy:    legacyConfig.HTTPConfig.NoProxy,
		},
	}
}
