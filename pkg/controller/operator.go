package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/rest"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/support-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/support-operator/pkg/controller/periodic"
	"github.com/openshift/support-operator/pkg/controller/status"
	"github.com/openshift/support-operator/pkg/gather"
	"github.com/openshift/support-operator/pkg/gather/clusterconfig"
	"github.com/openshift/support-operator/pkg/insights/insightsclient"
	"github.com/openshift/support-operator/pkg/insights/insightsuploader"
	"github.com/openshift/support-operator/pkg/record/diskrecorder"
)

type Support struct {
	StoragePath string
	Interval    time.Duration
	Endpoint    string
	Impersonate string
}

func (s *Support) LoadConfig(obj map[string]interface{}) error {
	var cfg struct {
		StoragePath *string `json:"storagePath"`
		Interval    *string `json:"interval"`
		Endpoint    *string `json:"endpoint"`
		Impersonate *string `json:"impersonate"`
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &cfg); err != nil {
		return fmt.Errorf("unable to load config: %v", err)
	}

	if cfg.Endpoint != nil {
		s.Endpoint = *cfg.Endpoint
	}
	if cfg.StoragePath != nil {
		s.StoragePath = *cfg.StoragePath
	}
	if cfg.Impersonate != nil {
		s.Impersonate = *cfg.Impersonate
	}
	if cfg.Interval != nil {
		d, err := time.ParseDuration(*cfg.Interval)
		if err != nil {
			return fmt.Errorf("interval must be a valid duration: %v", err)
		}
		s.Interval = d
	}

	if s.Interval <= 0 {
		return fmt.Errorf("interval must be a non-negative duration")
	}
	if len(s.StoragePath) == 0 {
		return fmt.Errorf("storagePath must point to a directory where snapshots can be stored")
	}

	data, _ := json.Marshal(s)
	klog.V(2).Infof("Current config:\n%s", string(data))

	return nil
}

func (s *Support) Run(controller *controllercmd.ControllerContext) error {
	klog.Infof("Starting support-operator %s", version.Get().String())

	if err := s.LoadConfig(controller.ComponentConfig.Object); err != nil {
		return err
	}

	ctx := context.Background()

	// these are operator clients
	client, err := kubernetes.NewForConfig(controller.ProtoKubeConfig)
	if err != nil {
		return err
	}
	configClient, err := configv1client.NewForConfig(controller.KubeConfig)
	if err != nil {
		return err
	}

	// these are gathering clients
	gatherCRKubeConfig := rest.CopyConfig(controller.KubeConfig)
	if len(s.Impersonate) > 0 {
		gatherCRKubeConfig.Impersonate.UserName = s.Impersonate
	}
	gatherConfigClient, err := configv1client.NewForConfig(gatherCRKubeConfig)
	if err != nil {
		return err
	}

	// ensure the support directory exists
	if _, err := os.Stat(s.StoragePath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(s.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// the status controller initializes the cluster operator object and retrieves
	// the last sync time, if any was set
	statusReporter := status.NewController(configClient)

	// the recorder periodically flushes any recorded data to disk as tar.gz files
	// in s.StoragePath, and also prunes files above a certain age
	recorder := diskrecorder.New(s.StoragePath, s.Interval)
	go recorder.PeriodicallyFlush(ctx)
	go recorder.PeriodicallyPrune(ctx, statusReporter)

	// the gatherers periodically check the state of the cluster and report any
	// config to the recorder
	configPeriodic := clusterconfig.New(gatherConfigClient)
	periodic := periodic.New(s.Interval, recorder, map[string]gather.Interface{
		"config": configPeriodic,
	})
	statusReporter.AddSources(periodic.Sources()...)
	go periodic.Run(4, ctx.Done())

	// endpoint is configured on the cluster but we can specify a default
	var insightsClient *insightsclient.Client
	if len(s.Endpoint) > 0 {
		authorizer := clusterauthorizer.New(client)
		if err := authorizer.Refresh(); err != nil {
			klog.Warningf("Unable to retrieve initial config: %v", err)
		}
		insightsClient = insightsclient.New(nil, s.Endpoint, 0, "default", authorizer, configPeriodic)
		// TODO: convert the authorizer refresh to a watch on the support config object once that
		// lands
		go authorizer.Run(ctx, wait.Jitter(2*time.Minute, 0.5))
	}

	// upload results to the provided client - if no client is configured reporting
	// is permanently disabled, but if a client does exist the server may still disable reporting
	uploader := insightsuploader.New(recorder, insightsClient, statusReporter, s.Interval*6)
	statusReporter.AddSources(uploader)
	uploader.Init()

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
