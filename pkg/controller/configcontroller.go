package controller

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type ConfigController struct {
	corev1Client *v1.CoreV1Client
	cmData       map[string]string
}

func NewConfigController(kubeConfig *rest.Config,
	eventRecorder events.Recorder,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces) (factory.Controller, error) {
	inf := kubeInformersForNamespaces.InformersFor("openshift-insights").Core().V1().ConfigMaps().Informer()
	corev1Cli, err := v1.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	c := &ConfigController{
		corev1Client: corev1Cli,
	}
	return factory.New().WithInformers(inf).
		WithSync(c.sync).
		ToController("InsightConfigController", eventRecorder), nil
}

func (c *ConfigController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	cmTest, err := c.corev1Client.ConfigMaps("openshift-insights").Get(ctx, "test", metav1.GetOptions{})
	if err != nil {
		return err
	}
	klog.Info("============================= Previously had ", c.cmData)
	klog.Info("============================= New has ", cmTest.Data)
	c.cmData = cmTest.Data
	return nil
}
