package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/authorizer/clusterauthorizer"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/periodic"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/insights"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsreport"
	"github.com/openshift/insights-operator/pkg/insights/insightsuploader"
	"github.com/openshift/insights-operator/pkg/ocm/clustertransfer"
	"github.com/openshift/insights-operator/pkg/ocm/sca"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
)

// Operator is the type responsible for controlling the start up of the Insights Operator
type Operator struct {
	config.Controller
}

// Run starts the Insights Operator:
// 1. Gets/Creates the necessary configs/clients
// 2. Starts the configobserver and status reporter
// 3. Initiates the recorder and starts the periodic record pruneing
// 4. Starts the periodic gathering
// 5. Creates the insights-client and starts uploader and reporter
func (s *Operator) Run(ctx context.Context, controller *controllercmd.ControllerContext) error { //nolint: funlen
	klog.Infof("Starting insights-operator %s", version.Get().String())
	initialDelay := 0 * time.Second
	cont, err := config.LoadConfig(s.Controller, controller.ComponentConfig.Object, config.ToController)
	if err != nil {
		return err
	}
	s.Controller = cont

	// Start the Prometheus metrics server.
	go insights.RunMetricsServer()

	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(controller.ProtoKubeConfig)
	if err != nil {
		return err
	}
	configClient, err := configv1client.NewForConfig(controller.KubeConfig)
	if err != nil {
		return err
	}

	gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig := prepareGatherConfigs(
		controller.ProtoKubeConfig, controller.KubeConfig, s.Impersonate,
	)

	// If we fail, it's likely due to the service CA not existing yet. Warn and continue,
	// and when the service-ca is loaded we will be restarted.
	_, err = kubernetes.NewForConfig(gatherProtoKubeConfig)
	if err != nil {
		return err
	}

	// ensure the insight snapshot directory exists
	if _, err = os.Stat(s.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(s.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(s.Controller, kubeClient)
	go configObserver.Start(ctx)

	// the status controller initializes the cluster operator object and retrieves
	// the last sync time, if any was set
	statusReporter := status.NewController(configClient, configObserver, os.Getenv("POD_NAMESPACE"))

	var anonymizer *anonymization.Anonymizer
	if anonymization.IsObfuscationEnabled(configObserver) {
		// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
		anonymizer, err = anonymization.NewAnonymizerFromConfig(ctx, gatherKubeConfig, gatherProtoKubeConfig, controller.ProtoKubeConfig)
		if err != nil {
			// in case of an error anonymizer will be nil and anonymization will be just skipped
			klog.Errorf(anonymization.UnableToCreateAnonymizerErrorMessage, err)
		}
	}

	// the recorder periodically flushes any recorded data to disk as tar.gz files
	// in s.StoragePath, and also prunes files above a certain age
	recdriver := diskrecorder.New(s.StoragePath)
	rec := recorder.New(recdriver, s.Interval, anonymizer)
	go rec.PeriodicallyPrune(ctx, statusReporter)

	authorizer := clusterauthorizer.New(configObserver)
	insightsClient := insightsclient.New(nil, 0, "default", authorizer, gatherKubeConfig)

	// the gatherers are periodically called to collect the data from the cluster
	// and provide the results for the recorder
	gatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configObserver, insightsClient,
	)
	periodicGather := periodic.New(configObserver, rec, gatherers, anonymizer)
	statusReporter.AddSources(periodicGather.Sources()...)

	// check we can read IO container status and we are not in crash loop
	initialCheckTimeout := s.Controller.Interval / 24
	initialCheckInterval := 20 * time.Second
	baseInitialDelay := s.Controller.Interval / 12
	err = wait.PollImmediate(initialCheckInterval, wait.Jitter(initialCheckTimeout, 0.1), isRunning(ctx, gatherKubeConfig))
	if err != nil {
		initialDelay = wait.Jitter(baseInitialDelay, 0.5)
		klog.Infof("Unable to check insights-operator pod status. Setting initial delay to %s", initialDelay)
	}
	go periodicGather.Run(ctx.Done(), initialDelay)

	// upload results to the provided client - if no client is configured reporting
	// is permanently disabled, but if a client does exist the server may still disable reporting
	uploader := insightsuploader.New(recdriver, insightsClient, configObserver, statusReporter, initialDelay)
	statusReporter.AddSources(uploader)

	// start reporting status now that all controller loops are added as sources
	if err = statusReporter.Start(ctx); err != nil {
		return fmt.Errorf("unable to set initial cluster status: %v", err)
	}
	// start uploading status, so that we
	// know any previous last reported time
	go uploader.Run(ctx)

	reportGatherer := insightsreport.New(insightsClient, configObserver, uploader)
	statusReporter.AddSources(reportGatherer)
	go reportGatherer.Run(ctx)

	scaController := initiateSCAController(ctx, kubeClient, configObserver, insightsClient)
	if scaController != nil {
		statusReporter.AddSources(scaController)
		go scaController.Run()
	}

	clusterTransferController := clustertransfer.New(ctx, kubeClient.CoreV1(), configObserver, insightsClient)
	statusReporter.AddSources(clusterTransferController)
	go clusterTransferController.Run()

	klog.Warning("started")

	<-ctx.Done()

	return nil
}

func isRunning(ctx context.Context, kubeConfig *rest.Config) wait.ConditionFunc {
	return func() (bool, error) {
		c, err := corev1client.NewForConfig(kubeConfig)
		if err != nil {
			return false, err
		}
		// check if context hasn't been canceled or done meanwhile
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
		for _, c := range pod.Status.ContainerStatuses { //nolint: gocritic
			// all containers has to be in running state to consider them healthy
			if c.LastTerminationState.Terminated != nil || c.LastTerminationState.Waiting != nil {
				klog.Info("The last pod state is unhealthy")
				return false, nil
			}
		}
		return true, nil
	}
}

// initiateSCAController creates a new sca.Controller
func initiateSCAController(ctx context.Context,
	kubeClient *kubernetes.Clientset, configObserver *configobserver.Controller, insightsClient *insightsclient.Client) *sca.Controller {
	// SCA controller periodically checks and pull data from the OCM SCA API
	// the data is exposed in the OpenShift API
	scaController := sca.New(ctx, kubeClient.CoreV1(), configObserver, insightsClient)
	return scaController
}
