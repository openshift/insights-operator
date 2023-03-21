package configobserver

import (
	"context"
	"sync"

	"github.com/openshift/api/config/v1alpha1"
	configCliv1alpha1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1alpha1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type APIConfigObserver interface {
	factory.Controller
	GatherConfig() *v1alpha1.GatherConfig
	GatherDataPolicy() *v1alpha1.DataPolicy
	GatherDisabled() bool
}

type APIConfigController struct {
	factory.Controller
	lock              sync.Mutex
	listeners         map[chan *v1alpha1.GatherConfig]struct{}
	configV1Alpha1Cli *configCliv1alpha1.ConfigV1alpha1Client
	gatherConfig      *v1alpha1.GatherConfig
}

func NewAPIConfigObserver(kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	configInformer configinformers.SharedInformerFactory) (APIConfigObserver, error) {
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
		klog.Errorf("Cannot read API gathering configuration: %v", err)
	}
	c.gatherConfig = &insightDataGatherConf.Spec.GatherConfig

	ctrl := factory.New().WithInformers(inf).
		WithSync(c.sync).
		ToController("InsightConfigController", eventRecorder)
	c.Controller = ctrl
	return c, nil
}

func (a *APIConfigController) sync(ctx context.Context, _ factory.SyncContext) error {
	insightDataGatherConf, err := a.configV1Alpha1Cli.InsightsDataGathers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}
	a.gatherConfig = &insightDataGatherConf.Spec.GatherConfig
	return nil
}

// GatherConfig provides the complete gather config in a thread-safe way.
func (a *APIConfigController) GatherConfig() *v1alpha1.GatherConfig {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.gatherConfig
}

// GatherDisabled tells whether data gathering is disabled or not
func (a *APIConfigController) GatherDisabled() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	if utils.StringInSlice("all", a.gatherConfig.DisabledGatherers) ||
		utils.StringInSlice("ALL", a.gatherConfig.DisabledGatherers) {
		return true
	}
	return false
}

// GatherDataPolicy provides DataPolicy attribute value defined in the API
func (a *APIConfigController) GatherDataPolicy() *v1alpha1.DataPolicy {
	a.lock.Lock()
	defer a.lock.Unlock()
	return &a.gatherConfig.DataPolicy
}
