package configobserver

import (
	"context"
	"sync"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configCliv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type InsightsDataGatherObserver interface {
	factory.Controller
	GatherConfig() *configv1.GatherConfig
	GatherDisabled() bool
}

type insightsDataGatherController struct {
	factory.Controller
	lock         sync.Mutex
	cli          configCliv1.ConfigV1Interface
	gatherConfig *configv1.GatherConfig
}

func NewInsightsDataGatherObserver(kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	configInformer configinformers.SharedInformerFactory,
) (InsightsDataGatherObserver, error) {
	inf := configInformer.Config().V1().InsightsDataGathers().Informer()
	configV1Cli, err := configCliv1.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	c := &insightsDataGatherController{
		cli: configV1Cli,
	}

	insightDataGatherConf, err := c.cli.InsightsDataGathers().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Cannot read API gathering configuration: %v", err)
	}
	c.gatherConfig = &insightDataGatherConf.Spec.GatherConfig

	ctrl := factory.New().WithInformers(inf).
		WithSync(c.sync).
		ResyncEvery(2*time.Minute).
		ToController("InsightsDataGatherObserver", eventRecorder)
	c.Controller = ctrl
	return c, nil
}

func (i *insightsDataGatherController) sync(ctx context.Context, _ factory.SyncContext) error {
	insightDataGatherConf, err := i.cli.InsightsDataGathers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}
	i.gatherConfig = &insightDataGatherConf.Spec.GatherConfig
	return nil
}

// GatherConfig provides the complete gather config in a thread-safe way.
func (i *insightsDataGatherController) GatherConfig() *configv1.GatherConfig {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.gatherConfig
}

// GatherDisabled checks if data gathering is disabled. This is true when
// the gathering mode is set to GatheringModeNone.
func (i *insightsDataGatherController) GatherDisabled() bool {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.gatherConfig == nil {
		return false
	}
	return i.gatherConfig.Gatherers.Mode == configv1.GatheringModeNone
}
