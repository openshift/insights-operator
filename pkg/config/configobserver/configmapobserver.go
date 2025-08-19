package configobserver

import (
	"context"
	"reflect"
	"sync"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	insightsConfigMapName = "insights-config"
)

type ConfigMapInformer interface {
	factory.Controller
	// Config provides actual Insights configuration values from the "insights-config" configmap
	Config() *config.InsightsConfiguration
	// ConfigChanged notifies all the listeners that the content of the "insights-config" configmap has changed
	ConfigChanged() (<-chan struct{}, func())
}

// ConfigMapObserver is a controller for "insights-config" config map
// in the "openshift-insights" namespace.
type ConfigMapObserver struct {
	factory.Controller
	lock           sync.Mutex
	kubeCli        *kubernetes.Clientset
	insightsConfig *config.InsightsConfiguration
	listeners      map[chan struct{}]struct{}
}

func NewConfigMapObserver(ctx context.Context, kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	kubeInformer v1helpers.KubeInformersForNamespaces,
) (ConfigMapInformer, error) {
	cmInformer := kubeInformer.InformersFor(insightsNamespaceName).Core().V1().ConfigMaps().Informer()
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	ctrl := &ConfigMapObserver{
		kubeCli:        kubeClient,
		insightsConfig: nil,
		listeners:      make(map[chan struct{}]struct{}),
	}
	factoryCtrl := factory.New().WithInformers(cmInformer).
		WithSync(ctrl.sync).
		ToController("ConfigController", eventRecorder)

	ctrl.Controller = factoryCtrl
	cm, err := getConfigMap(ctx, kubeClient)
	if err != nil {
		klog.Warningf("Cannot get the configuration config map: %v. Default configuration is used.", err)
		return ctrl, nil
	}
	insightsConfig, err := readConfigAndDecode(cm)
	if err != nil {
		klog.Warningf("Failed to read the configuration during start: %v. Default configuration is used.", err)
		return ctrl, nil
	}
	ctrl.insightsConfig = insightsConfig
	return ctrl, nil
}

// sync is called by the informer with every config map update
func (c *ConfigMapObserver) sync(ctx context.Context, _ factory.SyncContext) error {
	cm, err := getConfigMap(ctx, c.kubeCli)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		// config map doesn't exist so clear the config, notify and return
		klog.Info(err)
		// if the insightsConfig is nil then the map didn't exist and we don't need to notify
		if c.insightsConfig != nil {
			c.insightsConfig = nil
			c.notifyListeners()
		}
		return nil
	}
	insightsConfig, err := readConfigAndDecode(cm)
	if err != nil {
		return err
	}

	// config hasn't change - do nothing
	if reflect.DeepEqual(c.insightsConfig, insightsConfig) {
		return nil
	}
	c.insightsConfig = insightsConfig
	c.notifyListeners()
	return nil
}

func (c *ConfigMapObserver) notifyListeners() {
	for ch := range c.listeners {
		if ch == nil {
			continue
		}
		ch <- struct{}{}
	}
}

func (c *ConfigMapObserver) Config() *config.InsightsConfiguration {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.insightsConfig
}

func (c *ConfigMapObserver) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
	c.lock.Lock()
	defer c.lock.Unlock()
	ch := make(chan struct{})
	c.listeners[ch] = struct{}{}
	return ch, func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		close(ch)
		delete(c.listeners, ch)
	}
}

// readConfigAndDecode gets the "insights-config" config map and tries to decode its content. It returns
// "config.InsightsConfiguration" when successfully decoded, otherwise an error.
func readConfigAndDecode(cm *v1.ConfigMap) (*config.InsightsConfiguration, error) {
	insightsConfig := &config.InsightsConfigurationSerialized{}
	cfg := cm.Data["config.yaml"]
	err := yaml.Unmarshal([]byte(cfg), insightsConfig)
	if err != nil {
		return nil, err
	}
	return insightsConfig.ToConfig(), nil
}

func getConfigMap(ctx context.Context, kubeCli *kubernetes.Clientset) (*v1.ConfigMap, error) {
	return kubeCli.CoreV1().ConfigMaps(insightsNamespaceName).Get(ctx, insightsConfigMapName, metav1.GetOptions{})
}
