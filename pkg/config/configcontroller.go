package config

import (
	"context"
	"sync"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type APIObserver interface {
	factory.Controller
	ForceGather() (<-chan string, func())
}

type ConfigController struct {
	factory.Controller
	lock         sync.Mutex
	listeners    map[chan string]struct{}
	corev1Client *v1.CoreV1Client
	cmData       map[string]string
}

func NewConfigController(kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces) (APIObserver, error) {
	inf := kubeInformersForNamespaces.InformersFor("openshift-insights").Core().V1().ConfigMaps().Informer()
	corev1Cli, err := v1.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	c := &ConfigController{
		corev1Client: corev1Cli,
		listeners:    make(map[chan string]struct{}),
	}

	ctrl := factory.New().WithInformers(inf).
		WithSync(c.sync).
		ToController("InsightConfigController", eventRecorder)
	c.Controller = ctrl
	return c, nil
}

func (c *ConfigController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	cmTest, err := c.corev1Client.ConfigMaps("openshift-insights").Get(ctx, "test", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if fgr, ok := cmTest.Data["ForceGatherReason"]; ok {
		if fgr != c.cmData["ForceGatherReason"] && c.cmData["ForceGatherReason"] != "" {
			for ch := range c.listeners {
				if ch == nil {
					continue
				}
				ch <- fgr
			}
		}
	}
	c.cmData = cmTest.Data
	return nil
}

func (c *ConfigController) ForceGather() (configCh <-chan string, closeFn func()) {
	c.lock.Lock()
	defer c.lock.Unlock()
	ch := make(chan string, 1)
	c.listeners[ch] = struct{}{}
	return ch, func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		// close(ch)
		delete(c.listeners, ch)
	}
}
