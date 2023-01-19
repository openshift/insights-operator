package configobserver

import (
	"bytes"
	"context"
	"time"

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
	Config() *InsightsConfiguration
}

type ConfigMapController struct {
	factory.Controller
	kubeCli        *kubernetes.Clientset
	insightsConfig *InsightsConfiguration
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
	}
	factoryCtrl := factory.New().WithInformers(cmInformer).
		WithSync(ctrl.sync).
		ResyncEvery(10*time.Minute).
		ToController("ConfigController", eventRecorder)

	ctrl.Controller = factoryCtrl
	return ctrl, nil
}

func (c *ConfigMapController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	insightsConfig, err := readConfigAndDecode(ctx, c.kubeCli)
	if err != nil {
		return err
	}
	c.insightsConfig = insightsConfig
	return nil
}

func (c *ConfigMapController) Config() *InsightsConfiguration {
	return c.insightsConfig
}

func readConfigAndDecode(ctx context.Context, kubeCli *kubernetes.Clientset) (*InsightsConfiguration, error) {
	configCM, err := kubeCli.CoreV1().ConfigMaps("openshift-insights").Get(ctx, "insights-config", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	config := configCM.Data["config.yaml"]
	insightsConfig := &InsightsConfiguration{}
	err = yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer([]byte(config)), 1000).Decode(insightsConfig)
	if err != nil {
		return nil, err
	}
	return insightsConfig, nil
}
