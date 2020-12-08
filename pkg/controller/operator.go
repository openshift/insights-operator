package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/periodic"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gather/clusterconfig"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsreport"
	"github.com/openshift/insights-operator/pkg/insights/insightsuploader"
	"github.com/openshift/insights-operator/pkg/record/diskrecorder"
)

type Support struct {
	config.Controller
}

// LoadConfig unmarshalls config from obj and loads it to this Support struct
func (s *Support) LoadConfig(obj map[string]interface{}) error {
	var cfg config.Serialized
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &cfg); err != nil {
		return fmt.Errorf("unable to load config: %v", err)
	}

	controller, err := cfg.ToController(&s.Controller)
	if err != nil {
		return err
	}
	s.Controller = *controller

	data, _ := json.Marshal(cfg)
	klog.V(2).Infof("Current config: %s", string(data))
	return nil
}

func (s *Support) Run(ctx context.Context, controller *controllercmd.ControllerContext) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())
	initialDelay := 0 * time.Second
	if err := s.LoadConfig(controller.ComponentConfig.Object); err != nil {
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
	if len(s.Impersonate) > 0 {
		gatherProtoKubeConfig.Impersonate.UserName = s.Impersonate
	}
	gatherKubeConfig := rest.CopyConfig(controller.KubeConfig)
	if len(s.Impersonate) > 0 {
		gatherKubeConfig.Impersonate.UserName = s.Impersonate
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
	if _, err := os.Stat(s.StoragePath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(s.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(s.Controller, kubeClient)
	go configObserver.Start(ctx)

	// the status controller initializes the cluster operator object and retrieves
	// the last sync time, if any was set
	statusReporter := status.NewController(configClient, gatherKubeClient.CoreV1(), configObserver, os.Getenv("POD_NAMESPACE"))

	// the recorder periodically flushes any recorded data to disk as tar.gz files
	// in s.StoragePath, and also prunes files above a certain age
	recorder := diskrecorder.New(s.StoragePath, s.Interval)
	go recorder.PeriodicallyPrune(ctx, statusReporter)

	// the gatherers periodically check the state of the cluster and report any
	// config to the recorder
	clusterConfigGatherer := clusterconfig.New(gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig)
	periodic := periodic.New(configObserver, recorder, map[string]gather.Interface{
		"clusterconfig": clusterConfigGatherer,
	})
	statusReporter.AddSources(periodic.Sources()...)

	// check we can read IO container status and we are not in crash loop
	err = wait.PollImmediate(20*time.Second, wait.Jitter(s.Controller.Interval/12, 0.1), isRunning(ctx, gatherKubeConfig))
	if err != nil {
		initialDelay = wait.Jitter(s.Controller.Interval/12, 1)
		klog.Infof("Unable to check insights-operator pod status. Setting initial delay to %s", initialDelay)
	}
	go periodic.Run(4, ctx.Done(), initialDelay)

	authorizer := clusterauthorizer.New(configObserver)
	insightsClient := insightsclient.New(nil, 0, "default", authorizer, clusterConfigGatherer)

	// upload results to the provided client - if no client is configured reporting
	// is permanently disabled, but if a client does exist the server may still disable reporting
	uploader := insightsuploader.New(recorder, insightsClient, configObserver, statusReporter, initialDelay)
	statusReporter.AddSources(uploader)

	// TODO: future ideas
	//
	// * poll periodically for new insights commands to run, then delegate
	// * periodically dump crashlooping pod logs / save their messages
	// * watch cluster version for an upgrade, go into extra capture mode
	// * gather heap dumps from core components when master memory is above
	//   a threshold

	// start reporting status now that all controller loops are added as sources
	if err := statusReporter.Start(ctx); err != nil {
		return fmt.Errorf("unable to set initial cluster status: %v", err)
	}
	// start uploading status, so that we
	// know any previous last reported time
	go uploader.Run(ctx)

	reportGatherer := insightsreport.New(insightsClient, configObserver, uploader)
	go reportGatherer.Run(ctx)

	klog.Warning("stopped")

	<-ctx.Done()
	return nil
}

func isRunning(ctx context.Context, config *rest.Config) wait.ConditionFunc {
	return func() (bool, error) {
		c, err := corev1client.NewForConfig(config)
		if err != nil {
			return false, err
		}
		// check if context hasn't been cancelled or done meanwhile
		err = ctx.Err()
		if err != nil {
			return false, err
		}
		pod, err := c.Pods(os.Getenv("POD_NAMESPACE")).Get(ctx, os.Getenv("POD_NAME"), metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				klog.Errorf("Couldn't get Insights Operator Pod to detect its status. Error: %v", err)
			}
			return false, nil
		}
		for _, c := range pod.Status.ContainerStatuses {
			// all containers has to be in running state to consider them healthy
			if c.LastTerminationState.Terminated != nil || c.LastTerminationState.Waiting != nil {
				klog.Info("The last pod state is unhealthy")
				return false, nil
			}
		}
		return true, nil
	}
}
