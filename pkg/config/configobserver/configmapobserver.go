package configobserver

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ConfigMapObserver interface {
	factory.Controller
	// Config provides actual Insights configuration values from the "insights-config" configmap
	Config() *config.InsightsConfiguration
	// ConfigChanged notifies all the listeners that the content of the "insights-config" configmap has changed
	ConfigChanged() (<-chan struct{}, func())
}

// ConfigMapController is a controller for "insights-config" config map
// in openshift-insights namespace.
type ConfigMapController struct {
	factory.Controller
	lock           sync.Mutex
	kubeCli        *kubernetes.Clientset
	insightsConfig *config.InsightsConfiguration
	listeners      map[chan struct{}]struct{}
}

func NewConfigObserver(ctx context.Context, kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	kubeInformer v1helpers.KubeInformersForNamespaces) (ConfigMapObserver, error) {
	cmInformer := kubeInformer.InformersFor("openshift-insights").Core().V1().ConfigMaps().Informer()
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	insightsConfig, err := readConfigAndDecode(ctx, kubeClient)
	if err != nil {
		return nil, err
	}
	ctrl := &ConfigMapController{
		kubeCli:        kubeClient,
		insightsConfig: insightsConfig,
		listeners:      make(map[chan struct{}]struct{}),
	}
	factoryCtrl := factory.New().WithInformers(cmInformer).
		WithSync(ctrl.sync).
		ResyncEvery(10*time.Minute).
		ToController("ConfigController", eventRecorder)

	ctrl.Controller = factoryCtrl
	return ctrl, nil
}

// sync is called by the informer with every config map update
func (c *ConfigMapController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	insightsConfig, err := readConfigAndDecode(ctx, c.kubeCli)
	if err != nil {
		return err
	}
	// do not notify listeners on resync
	if *c.insightsConfig != *insightsConfig {
		for ch := range c.listeners {
			if ch == nil {
				continue
			}
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		c.insightsConfig = insightsConfig
	}
	return nil
}

func (c *ConfigMapController) Config() *config.InsightsConfiguration {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.insightsConfig
}

func (c *ConfigMapController) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
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

// readConfigAndDecode gets the "insights-config" config map and tries to decode its content. It returns
// "config.InsightsConfiguration" when successfully decoded, otherwise an error.
func readConfigAndDecode(ctx context.Context, kubeCli *kubernetes.Clientset) (*config.InsightsConfiguration, error) {
	configCM, err := kubeCli.CoreV1().ConfigMaps("openshift-insights").Get(ctx, "insights-config", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	cfg := configCM.Data["config.yaml"]
	insightsConfig := &config.InsightsConfigurationSerialized{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer([]byte(cfg)), 1000).Decode(insightsConfig)
	if err != nil {
		return nil, err
	}
	return insightsConfig.ToConfig(), nil
}
