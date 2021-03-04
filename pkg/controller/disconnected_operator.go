package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/periodic"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gather/clusterconfig"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
)

type Disconnected struct {
	config.Controller
}

// LoadConfig unmarshalls config from obj and loads it to this Support struct
func (d *Disconnected) LoadConfig(obj map[string]interface{}) error {
	var cfg config.Serialized
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &cfg); err != nil {
		return fmt.Errorf("unable to load config: %v", err)
	}

	controller, err := cfg.ToSimpleController(&d.Controller)
	if err != nil {
		return err
	}
	d.Controller = *controller

	data, _ := json.Marshal(cfg)
	klog.V(2).Infof("Current config: %s", string(data))
	return nil
}

func (d *Disconnected) Run(ctx context.Context, controller *controllercmd.ControllerContext) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())
	initialDelay := 0 * time.Second
	if err := d.LoadConfig(controller.ComponentConfig.Object); err != nil {
		return err
	}

	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(controller.ProtoKubeConfig)
	if err != nil {
		return err
	}
	configClient, err := configv1client.NewForConfig(controller.KubeConfig)
	if err != nil {
		return err
	}
	// these are gathering clients
	gatherProtoKubeConfig := rest.CopyConfig(controller.ProtoKubeConfig)
	if len(d.Impersonate) > 0 {
		gatherProtoKubeConfig.Impersonate.UserName = d.Impersonate
	}
	gatherKubeConfig := rest.CopyConfig(controller.KubeConfig)
	if len(d.Impersonate) > 0 {
		gatherKubeConfig.Impersonate.UserName = d.Impersonate
	}

	// the metrics client will connect to prometheus and scrape a small set of metrics
	// TODO: the oauth-proxy and delegating authorizer do not support Impersonate-User,
	//   so we do not impersonate gather
	metricsGatherKubeConfig := rest.CopyConfig(controller.KubeConfig)
	metricsGatherKubeConfig.CAFile = "/var/run/configmaps/service-ca-bundle/service-ca.crt"
	metricsGatherKubeConfig.NegotiatedSerializer = scheme.Codecs
	metricsGatherKubeConfig.GroupVersion = &schema.GroupVersion{}
	metricsGatherKubeConfig.APIPath = "/"
	metricsGatherKubeConfig.Host = "https://prometheus-k8s.openshift-monitoring.svc:9091"

	// If we fail, it's likely due to the service CA not existing yet. Warn and continue,
	// and when the service-ca is loaded we will be restarted.
	gatherKubeClient, err := kubernetes.NewForConfig(gatherProtoKubeConfig)
	if err != nil {
		return err
	}
	// ensure the insight snapshot directory exists
	if _, err := os.Stat(d.StoragePath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(d.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(d.Controller, kubeClient)
	go configObserver.Start(ctx)

	// the status controller initializes the cluster operator object and retrieves
	// the last sync time, if any was set
	statusReporter := status.NewController(configClient, gatherKubeClient.CoreV1(), configObserver, os.Getenv("POD_NAMESPACE"))

	// the recorder periodically flushes any recorded data to disk as tar.gz files
	// in s.StoragePath, and also prunes files above a certain age
	recdriver := diskrecorder.New(d.StoragePath)
	recorder := recorder.New(recdriver, d.Interval)
	go recorder.PeriodicallyPrune(ctx, statusReporter)

	// the gatherers periodically check the state of the cluster and report any
	// config to the recorder
	clusterConfigGatherer := clusterconfig.New(gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig)
	periodic := periodic.New(configObserver, recorder, map[string]gather.Interface{
		"clusterconfig": clusterConfigGatherer,
	})
	statusReporter.AddSources(periodic.Sources()...)

	// check we can read IO container status and we are not in crash loop
	err = wait.PollImmediate(20*time.Second, wait.Jitter(d.Controller.Interval/24, 0.1), isRunning(ctx, gatherKubeConfig))
	if err != nil {
		initialDelay = wait.Jitter(d.Controller.Interval/12, 0.5)
		klog.Infof("Unable to check insights-operator pod status. Setting initial delay to %s", initialDelay)
	}
	go periodic.Run(ctx.Done(), initialDelay)

	klog.Warning("stopped")

	<-ctx.Done()
	return nil
}
