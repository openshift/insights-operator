package configobserver

import (
	"context"
	"sync"
	"time"

	"github.com/openshift/api/config/v1alpha2"
	configCliv1alpha2 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1alpha2"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type InsightsDataGatherObserver interface {
	factory.Controller
	GatherConfig() *v1alpha2.GatherConfig
	GatherDisabled() bool
}

type insightsDataGatherController struct {
	factory.Controller
	lock         sync.Mutex
	cli          configCliv1alpha2.ConfigV1alpha2Interface
	gatherConfig *v1alpha2.GatherConfig
}

func NewInsightsDataGatherObserver(kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	configInformer configinformers.SharedInformerFactory,
) (InsightsDataGatherObserver, error) {
	inf := configInformer.Config().V1alpha2().InsightsDataGathers().Informer()
	configV1Alpha2Cli, err := configCliv1alpha2.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	c := &insightsDataGatherController{
		cli: configV1Alpha2Cli,
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
func (i *insightsDataGatherController) GatherConfig() *v1alpha2.GatherConfig {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.gatherConfig
}

// GatherDisabled tells whether data gathering is disabled or not
func (i *insightsDataGatherController) GatherDisabled() bool {
	i.lock.Lock()
	defer i.lock.Unlock()

	return i.gatherConfig.Gatherers.Mode == v1alpha2.GatheringModeNone
}
