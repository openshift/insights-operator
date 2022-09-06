package config

import (
	"context"
	"fmt"
	"sync"

	"github.com/openshift/api/config/v1alpha1"
	configCliv1alpha1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1alpha1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type APIObserver interface {
	factory.Controller
	GatherConfig() *v1alpha1.GatherConfig
}

type APIConfigController struct {
	factory.Controller
	lock              sync.Mutex
	listeners         map[chan *v1alpha1.GatherConfig]struct{}
	configV1Alpha1Cli *configCliv1alpha1.ConfigV1alpha1Client
	gatherConfig      *v1alpha1.GatherConfig
}

func NewConfigController(kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	configInformer configinformers.SharedInformerFactory) (APIObserver, error) {
	inf := configInformer.Config().V1alpha1().InsightsDataGathers().Informer()
	configV1Alpha1Cli, err := configCliv1alpha1.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	c := &APIConfigController{
		configV1Alpha1Cli: configV1Alpha1Cli,
		listeners:         make(map[chan *v1alpha1.GatherConfig]struct{}),
	}

	insightDataGatherConf, err := c.configV1Alpha1Cli.InsightsDataGathers().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		klog.Error("Cannot read API gathering configuration: %v", err)
	}
	c.gatherConfig = &insightDataGatherConf.Spec.GatherConfig

	ctrl := factory.New().WithInformers(inf).
		WithSync(c.sync).
		ToController("InsightConfigController", eventRecorder)
	c.Controller = ctrl
	return c, nil
}

func (c *APIConfigController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	insightDataGatherConf, err := c.configV1Alpha1Cli.InsightsDataGathers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}
	fmt.Println("===================================== DISABLED GATHERERS ", insightDataGatherConf.Spec.GatherConfig.DisabledGatherers)
	c.gatherConfig = &insightDataGatherConf.Spec.GatherConfig
	for ch := range c.listeners {
		if ch == nil {
			continue
		}
		select {
		case ch <- &insightDataGatherConf.Spec.GatherConfig:
		default:
		}
	}
	return nil
}

/* func (c *APIConfigController) GatherConfig() (configCh <-chan *v1alpha1.GatherConfig, closeFn func()) {
	c.lock.Lock()
	defer c.lock.Unlock()
	ch := make(chan *v1alpha1.GatherConfig, 1)
	c.listeners[ch] = struct{}{}
	return ch, func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		close(ch)
		delete(c.listeners, ch)
	}
}
*/

// Config provides the config in a thread-safe way.
func (c *APIConfigController) GatherConfig() *v1alpha1.GatherConfig {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.gatherConfig
}
