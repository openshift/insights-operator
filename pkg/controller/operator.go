package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openshift/api/features"
	insightsv1alpha2 "github.com/openshift/api/insights/v1alpha2"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	configv1informers "github.com/openshift/client-go/config/informers/externalversions"
	insightsv1alpha1client "github.com/openshift/client-go/insights/clientset/versioned"
	"github.com/openshift/client-go/insights/clientset/versioned/typed/insights/v1alpha2"
	insightsInformers "github.com/openshift/client-go/insights/informers/externalversions"
	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"
	operatorinformers "github.com/openshift/client-go/operator/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientInformers "k8s.io/client-go/informers"
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
	"github.com/openshift/library-go/pkg/operator/loglevel"
)

const (
	insightsNamespace = "openshift-insights"
	informerTimeout   = 10 * time.Minute
)

// TechPreviewInformers holds all informers needed for TechPreview functionality
// when the InsightsConfig feature gate is enabled.
type TechPreviewInformers struct {
	JobInformer                periodic.JobWatcher
	InsightsDataGatherObserver configobserver.InsightsDataGatherObserver
	DataGatherInformer         periodic.DataGatherInformer
}

// Operator is the type responsible for controlling the start-up of the Insights Operator
type Operator struct {
	config.Controller
}

// Run starts the Insights Operator:
// 1. Gets/Creates the necessary configs/clients
// 2. Starts the configobserver and status reporter
// 3. Initiates the recorder and starts the periodic record pruneing
// 4. Starts the periodic gathering
// 5. Creates the insights-client and starts uploader and reporter
func (s *Operator) Run(ctx context.Context, controller *controllercmd.ControllerContext) error { //nolint: funlen, gocyclo
	klog.Infof("Starting insights-operator %s", version.Get().String())
	initialDelay := 0 * time.Second
	cont, err := config.LoadConfig(s.Controller, controller.ComponentConfig.Object, config.ToController)
	if err != nil {
		return err
	}
	s.Controller = cont

	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(controller.ProtoKubeConfig)
	if err != nil {
		return err
	}
	configClient, err := configv1client.NewForConfig(controller.KubeConfig)
	if err != nil {
		return err
	}
	configInformers := configv1informers.NewSharedInformerFactory(configClient, informerTimeout)

	operatorClient, err := operatorclient.NewForConfig(controller.KubeConfig)
	if err != nil {
		return err
	}

	insightClient, err := insightsv1alpha1client.NewForConfig(controller.KubeConfig)
	if err != nil {
		return err
	}

	operatorConfigInformers := operatorinformers.NewSharedInformerFactory(operatorClient, informerTimeout)

	opClient := &genericClient{
		informers: operatorConfigInformers,
		client:    operatorClient.OperatorV1(),
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

	missingVersion := "0.0.1-snapshot"
	desiredVersion := missingVersion
	if envVersion, exists := os.LookupEnv("RELEASE_VERSION"); exists {
		desiredVersion = envVersion
	}

	// By default, this will exit(0) the process if the featuregates ever change to a different set of values.
	featureGateAccessor := featuregates.NewFeatureGateAccess(
		desiredVersion, missingVersion,
		configInformers.Config().V1().ClusterVersions(), configInformers.Config().V1().FeatureGates(),
		controller.EventRecorder,
	)
	go featureGateAccessor.Run(ctx)
	go configInformers.Start(ctx.Done())

	select {
	case <-featureGateAccessor.InitialFeatureGatesObserved():
		featureGates, _ := featureGateAccessor.CurrentFeatureGates()
		klog.Infof("FeatureGates initialized: knownFeatureGates=%v", featureGates.KnownFeatures())
	case <-time.After(1 * time.Minute):
		klog.Errorf("timed out waiting for FeatureGate detection")
		return fmt.Errorf("timed out waiting for FeatureGate detection")
	}

	featureGates, err := featureGateAccessor.CurrentFeatureGates()
	if err != nil {
		return err
	}

	insightsConfigEnabled := featureGates.Enabled(features.FeatureGateInsightsConfig)

	// ensure the insight snapshot directory exists
	if _, err = os.Stat(s.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(s.StoragePath, 0o777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	var techPreviewInformers *TechPreviewInformers
	if insightsConfigEnabled {
		deleteAllRunningGatheringsPods(ctx, kubeClient, insightClient.InsightsV1alpha2())

		techPreviewInformers, err = createTechPreviewInformers(
			ctx,
			kubeClient,
			configClient,
			insightClient,
			gatherKubeConfig,
			controller.EventRecorder,
		)
		if err != nil {
			return fmt.Errorf("failed to create TechPreview informers: %w", err)
		}
	}

	kubeInf := v1helpers.NewKubeInformersForNamespaces(kubeClient, "openshift-insights")
	configMapObserver, err := configobserver.NewConfigMapObserver(ctx, gatherKubeConfig, controller.EventRecorder, kubeInf)
	if err != nil {
		return err
	}
	go kubeInf.Start(ctx.Done())
	go configMapObserver.Run(ctx, 1)

	// secretConfigObserver synthesizes all config into the status reporter controller
	secretConfigObserver := configobserver.New(s.Controller, kubeClient)
	go secretConfigObserver.Start(ctx)

	configAggregator := configobserver.NewConfigAggregator(secretConfigObserver, configMapObserver)
	go configAggregator.Listen(ctx)

	// the status controller initializes the cluster operator object and retrieves
	// the last sync time, if any was set
	statusReporter := status.NewController(configClient.ConfigV1(), configAggregator,
		techPreviewInformers.InsightsDataGatherObserver, os.Getenv("POD_NAMESPACE"), insightsConfigEnabled)

	var anonymizer *anonymization.Anonymizer
	var recdriver *diskrecorder.DiskRecorder
	var rec *recorder.Recorder
	// if techPreview is enabled we switch to separate job and we don't need anything from this
	if !insightsConfigEnabled {
		// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
		anonymizer, err = anonymization.NewAnonymizerFromConfig(ctx, gatherKubeConfig,
			gatherProtoKubeConfig, controller.ProtoKubeConfig, configAggregator, []insightsv1alpha2.DataPolicyOption{})
		if err != nil {
			// in case of an error anonymizer will be nil and anonymization will be just skipped
			klog.Errorf(anonymization.UnableToCreateAnonymizerErrorMessage, err)
			return err
		}

		// the recorder periodically flushes any recorded data to disk as tar.gz files
		// in s.StoragePath, and also prunes files above a certain age
		recdriver = diskrecorder.New(s.StoragePath)
		rec = recorder.New(recdriver, s.Interval, anonymizer)
		go rec.PeriodicallyPrune(ctx, statusReporter)
	}

	authorizer := clusterauthorizer.New(secretConfigObserver, configAggregator)

	// gatherConfigClient is configClient created from gatherKubeConfig, this name was used because configClient was already taken
	// this client is only used in insightsClient, it is created here
	// because pkg/insights/insightsclient/request_test.go unit test won't work otherwise
	gatherConfigClient, err := configv1client.NewForConfig(gatherKubeConfig)
	if err != nil {
		return err
	}

	insightsClient := insightsclient.New(nil, 0, "insights", authorizer, gatherConfigClient)

	var periodicGather *periodic.Controller
	// the gatherers are periodically called to collect the data from the cluster
	// and provide the results for the recorder
	gatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig, anonymizer,
		configAggregator, insightsClient,
	)
	if !insightsConfigEnabled {
		periodicGather = periodic.New(configAggregator, rec, gatherers, anonymizer,
			operatorClient.OperatorV1().InsightsOperators(), kubeClient)
		statusReporter.AddSources(periodicGather.Sources()...)
	} else {
		reportRetriever := insightsreport.NewWithTechPreview(insightsClient, configAggregator)
		periodicGather = periodic.NewWithTechPreview(
			reportRetriever,
			configAggregator,
			techPreviewInformers.InsightsDataGatherObserver,
			gatherers,
			kubeClient, insightClient.InsightsV1alpha2(), operatorClient.OperatorV1().InsightsOperators(), configClient.ConfigV1(),
			techPreviewInformers.DataGatherInformer, techPreviewInformers.JobInformer)
		statusReporter.AddSources(periodicGather.Sources()...)
		statusReporter.AddSources(reportRetriever)
		go periodicGather.PeriodicPrune(ctx)
	}

	// check we can read IO container status, and we are not in crash loop
	initialCheckTimeout := s.Controller.Interval / 24
	initialCheckInterval := 20 * time.Second
	baseInitialDelay := s.Controller.Interval / 12
	err = wait.PollUntilContextTimeout(ctx, initialCheckInterval, wait.Jitter(initialCheckTimeout, 0.1), true, isRunning(gatherKubeConfig))
	if err != nil {
		initialDelay = wait.Jitter(baseInitialDelay, 0.5)
		klog.Infof("Unable to check insights-operator pod status. Setting initial delay to %s", initialDelay)
	}
	go periodicGather.Run(ctx.Done(), initialDelay)

	if !insightsConfigEnabled {
		// upload results to the provided client - if no client is configured reporting
		// is permanently disabled, but if a client does exist the server may still disable reporting
		uploader := insightsuploader.New(recdriver, insightsClient, configAggregator,
			techPreviewInformers.InsightsDataGatherObserver, statusReporter, initialDelay)
		statusReporter.AddSources(uploader)

		// start uploading status, so that we
		// know any previous last reported time
		go uploader.Run(ctx, initialDelay)

		reportGatherer := insightsreport.New(insightsClient, configAggregator, uploader, operatorClient.OperatorV1().InsightsOperators())
		statusReporter.AddSources(reportGatherer)
		go reportGatherer.Run(ctx)
	}

	// start reporting status now that all controller loops are added as sources
	if err = statusReporter.Start(ctx); err != nil {
		return fmt.Errorf("unable to set initial cluster status: %v", err)
	}

	scaController := sca.New(kubeClient.CoreV1(), configAggregator, insightsClient)
	statusReporter.AddSources(scaController)
	go scaController.Run(ctx)

	clusterTransferController := clustertransfer.New(kubeClient.CoreV1(), configAggregator, insightsClient)
	statusReporter.AddSources(clusterTransferController)
	go clusterTransferController.Run(ctx)

	promRulesController := insights.NewPrometheusRulesController(configAggregator, controller.KubeConfig)
	go promRulesController.Start(ctx)

	// support logLevelController
	logLevelController := loglevel.NewClusterOperatorLoggingController(opClient, controller.EventRecorder)
	operatorConfigInformers.Start(ctx.Done())
	go logLevelController.Run(ctx, 1)
	klog.Warning("started")

	<-ctx.Done()

	return nil
}

func isRunning(kubeConfig *rest.Config) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
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

// createTechPreviewInformers creates and starts all informers needed for TechPreview functionality.
func createTechPreviewInformers(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	configClient *configv1client.Clientset,
	insightClient *insightsv1alpha1client.Clientset,
	gatherKubeConfig *rest.Config,
	eventRecorder events.Recorder,
) (*TechPreviewInformers, error) {
	// Create Job informer for watching gathering job completions
	sharedInformer := clientInformers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		informerTimeout,
		clientInformers.WithNamespace(insightsNamespace),
		// Watch only jobs with a given label
		clientInformers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = "insights-gathering"
		}))
	jobInformer, err := periodic.NewJobCompletionWatcher(eventRecorder, sharedInformer)
	if err != nil {
		return nil, fmt.Errorf("failed to create job informer: %w", err)
	}

	// Create InsightsDataGather observer for global configuration
	configInformersForTechPreview := configv1informers.NewSharedInformerFactory(configClient, informerTimeout)
	insightsDataGatherObserver, err := configobserver.NewInsightsDataGatherObserver(gatherKubeConfig,
		eventRecorder, configInformersForTechPreview)
	if err != nil {
		return nil, fmt.Errorf("failed to create InsightsDataGather observer: %w", err)
	}

	// Create DataGather informer for watching DataGather CRs
	insightsInformersfactory := insightsInformers.NewSharedInformerFactory(insightClient, informerTimeout)
	dgInformer, err := periodic.NewDataGatherInformer(eventRecorder, insightsInformersfactory)
	if err != nil {
		return nil, fmt.Errorf("failed to create DataGather informer: %w", err)
	}

	// Start all informers
	go jobInformer.Run(ctx, 1)
	go sharedInformer.Start(ctx.Done())
	go insightsDataGatherObserver.Run(ctx, 1)
	go configInformersForTechPreview.Start(ctx.Done())
	go dgInformer.Run(ctx, 1)
	go insightsInformersfactory.Start(ctx.Done())

	return &TechPreviewInformers{
		JobInformer:                jobInformer,
		InsightsDataGatherObserver: insightsDataGatherObserver,
		DataGatherInformer:         dgInformer,
	}, nil
}

// deleteAllRunningGatheringsPods deletes all the active jobs (and their Pods) with the "periodic-gathering-"
// prefix in the openshift-insights namespace
func deleteAllRunningGatheringsPods(
	ctx context.Context, cli kubernetes.Interface, insightClient v1alpha2.InsightsV1alpha2Interface,
) {
	jobList, err := cli.BatchV1().Jobs(insightsNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list jobs in the %s namespace: %v", insightsNamespace, err)
		return
	}

	orphan := metav1.DeletePropagationBackground
	for i := range jobList.Items {
		j := jobList.Items[i]
		if j.Status.Active > 0 && strings.HasPrefix(j.Name, "periodic-gathering-") {
			klog.Infof("Deleting active gathering job %s due to operator restart", j.Name)

			err := cli.BatchV1().Jobs(insightsNamespace).Delete(ctx, j.Name, metav1.DeleteOptions{
				PropagationPolicy: &orphan,
			})
			if err != nil {
				klog.Errorf("Failed to delete job %s in namespace %s: %v", j.Name, insightsNamespace, err)
				continue
			}

			if _, err := status.UpdateProgressingCondition(
				ctx,
				insightClient,
				nil,
				j.Name,
				status.GatheringFailedReason,
			); err != nil {
				klog.Warningf("Failed to update DataGather CR status for job %s after deletion due to operator restart: %v",
					j.Name, err)
			}
		}
	}
}
