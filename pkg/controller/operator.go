package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"k8s.io/klog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"

	apiconfigv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/insights-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/periodic"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/gather/clusterconfig"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsuploader"
	"github.com/openshift/insights-operator/pkg/record/diskrecorder"
)

type Support struct {
	config.Controller
}

func (s *Support) LoadConfig(obj map[string]interface{}) error {
	var cfg config.Serialized
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &cfg); err != nil {
		return fmt.Errorf("unable to load config: %v", err)
	}
	controller, err := cfg.ToController()
	if err != nil {
		return err
	}
	s.Controller = *controller

	data, _ := json.Marshal(cfg)
	klog.V(2).Infof("Current config: %s", string(data))

	return nil
}

func (s *Support) Run(controller *controllercmd.ControllerContext) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())

	if err := s.LoadConfig(controller.ComponentConfig.Object); err != nil {
		return err
	}

	ctx := context.Background()

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
	gatherKubeClient, err := kubernetes.NewForConfig(gatherProtoKubeConfig)
	if err != nil {
		return err
	}
	gatherConfigClient, err := configv1client.NewForConfig(gatherKubeConfig)
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
	statusReporter := status.NewController(configClient, configObserver, os.Getenv("POD_NAMESPACE"))

	// the recorder periodically flushes any recorded data to disk as tar.gz files
	// in s.StoragePath, and also prunes files above a certain age
	recorder := diskrecorder.New(s.StoragePath, s.Interval)
	go recorder.PeriodicallyPrune(ctx, statusReporter)

	// the gatherers periodically check the state of the cluster and report any
	// config to the recorder
	configPeriodic := clusterconfig.New(gatherConfigClient, gatherKubeClient.CoreV1())
	periodic := periodic.New(s.Interval, recorder, map[string]gather.Interface{
		"config": configPeriodic,
	})
	statusReporter.AddSources(periodic.Sources()...)
	go periodic.Run(4, ctx.Done())

	authorizer := clusterauthorizer.New(configObserver)
	insightsClient := insightsclient.New(nil, 0, "default", authorizer, configPeriodic)

	// upload results to the provided client - if no client is configured reporting
	// is permanently disabled, but if a client does exist the server may still disable reporting
	uploader := insightsuploader.New(recorder, insightsClient, configObserver, statusReporter)
	statusReporter.AddSources(uploader)

	// Instrumentation goroutine initialization logic
	if enabled := os.Getenv("IO_ENABLE_INSTRUMENTATION"); enabled == "true" {
		go func(ctrlCtx *controllercmd.ControllerContext, interval time.Duration) {
			clusterID, err := func(ctrlCtx *controllercmd.ControllerContext) (apiconfigv1.ClusterID, error) {
				instrClient, err := configv1client.NewForConfig(ctrlCtx.KubeConfig)
				if err != nil {
					return "", err
				}

				instrClusterVersion, err := instrClient.ClusterVersions().Get("version", metav1.GetOptions{})
				if err != nil {
					return "", err
				}

				if instrClusterVersion == nil {
					return "", errors.New("cluster version is nil")
				}

				return instrClusterVersion.Spec.ClusterID, nil
			}(ctrlCtx)

			klog.Infoln("Cluster ID:", clusterID, "| Error:", err)

			for {
				// TODO: Replace with a request to the instrumentation service.
				// Just to mock that sometimes running must-gather is requested and sometimes it's not.
				mustGatherRequested := time.Now().Unix()%10 == 0

				klog.Infoln("Insights operator instrumentation is running")

				if mustGatherRequested {
					klog.Infoln("Must-gather requested")
					// TODO: Run must-gather.
					// Emulates delay caused by running must-gather.
					// The real delay will most likely be upwards of 5 minutes.
					time.Sleep(10 * time.Second)
					klog.Infoln("Must-gather finished")
				}

				time.Sleep(interval)
			}
		}(controller, time.Minute)
	} else {
		klog.Infoln("Insights operator instrumentation is disabled")
	}

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

	<-ctx.Done()
	return fmt.Errorf("stopped")
}
