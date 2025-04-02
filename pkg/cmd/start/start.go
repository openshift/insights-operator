package start

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openshift/api/features"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	configv1informers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/serviceability"

	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller"
)

const (
	serviceCACertPath    = "/var/run/configmaps/service-ca-bundle/service-ca.crt"
	pbContentType        = "application/vnd.kubernetes.protobuf"
	pbAcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	missingVersion       = "0.0.1-snapshot"
)

// NewOperator create the command for running the Insights Operator.
func NewOperator() *cobra.Command {
	operator := &controller.Operator{
		Controller: config.Controller{
			StoragePath:                 "/var/lib/insights-operator",
			Interval:                    10 * time.Minute,
			Endpoint:                    "https://console.redhat.com/api/ingress/v1/upload",
			ReportEndpoint:              "https://console.redhat.com/api/insights-results-aggregator/v2/cluster/%s/reports",
			ConditionalGathererEndpoint: "https://console.redhat.com/api/gathering/gathering_rules",
			ReportPullingDelay:          60 * time.Second,
			ReportMinRetryTime:          10 * time.Second,
			ReportPullingTimeout:        30 * time.Minute,
			OCMConfig: config.OCMConfig{
				SCAInterval:             8 * time.Hour,
				SCAEndpoint:             "https://api.openshift.com/api/accounts_mgmt/v1/certificates",
				ClusterTransferEndpoint: "https://api.openshift.com/api/accounts_mgmt/v1/cluster_transfers",
				ClusterTransferInterval: 12 * time.Hour,
			},
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), operator.Run, clock.RealClock{})
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the operator",
		Run:   runOperator(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommandWithContext(context.Background()).Flags())

	return cmd
}

// NewGather create the command for running a single gather.
func NewGather() *cobra.Command {
	operator := &controller.GatherJob{
		Controller: config.Controller{
			ConditionalGathererEndpoint: "https://console.redhat.com/api/gathering/gathering_rules",
			StoragePath:                 "/var/lib/insights-operator",
			Interval:                    30 * time.Minute,
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), nil, clock.RealClock{})
	cmd := &cobra.Command{
		Use:   "gather",
		Short: "Does a single gather, without uploading it",
		Run:   runGather(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommandWithContext(context.Background()).Flags())

	return cmd
}

func NewGatherAndUpload() *cobra.Command {
	operator := &controller.GatherJob{
		Controller: config.Controller{
			ConditionalGathererEndpoint: "https://console.redhat.com/api/gathering/gathering_rules",
			StoragePath:                 "/var/lib/insights-operator",
			Interval:                    2 * time.Hour,
			Endpoint:                    "https://console.redhat.com/api/ingress/v1/upload",
			ReportEndpoint:              "https://console.redhat.com/api/insights-results-aggregator/v2/cluster/%s/reports",
			ReportPullingDelay:          60 * time.Second,
			ReportMinRetryTime:          10 * time.Second,
			ReportPullingTimeout:        30 * time.Minute,
			ProcessingStatusEndpoint:    "https://console.redhat.com/api/insights-results-aggregator/v2/cluster/%s/request/%s/status",
			ReportEndpointTechPreview:   "https://console.redhat.com/api/insights-results-aggregator/v2/cluster/%s/request/%s/report",
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), nil, clock.RealClock{})
	cmd := &cobra.Command{
		Use:   "gather-and-upload",
		Short: "Runs the data gathering as job, uploads the data, waits for Insights analysis report and ends",
		Run:   runGatherAndUpload(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommand().Flags())

	return cmd
}

// Boilerplate for running an operator and handling command line arguments.
func runOperator(operator *controller.Operator, cfg *controllercmd.ControllerCommandConfig) func(cmd *cobra.Command, _ []string) {
	return func(cmd *cobra.Command, _ []string) {
		// boilerplate for the "normal" command
		defer serviceability.BehaviorOnPanic(os.Getenv("OPENSHIFT_ON_PANIC"), version.Get())()
		defer serviceability.Profile(os.Getenv("OPENSHIFT_PROFILE")).Stop()
		serviceability.StartProfiler()

		if configArg := cmd.Flags().Lookup("config").Value.String(); len(configArg) == 0 {
			klog.Exit("error: --config is required")
		}

		unstructured, operatorConfig, configBytes, err := cfg.Config()
		if err != nil {
			klog.Exit(err)
		}

		startingFileContent, observedFiles, err := cfg.AddDefaultRotationToConfig(operatorConfig, configBytes)
		if err != nil {
			klog.Exit(err)
		}

		// if the service CA is rotated, we want to restart
		if data, err := os.ReadFile(serviceCACertPath); err == nil {
			startingFileContent[serviceCACertPath] = data
		} else {
			klog.Infof("Unable to read service ca bundle: %v", err)
		}
		observedFiles = append(observedFiles, serviceCACertPath)

		exitOnChangeReactorCh := make(chan struct{})
		ctx := context.Background()
		ctx2, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-exitOnChangeReactorCh:
				cancel()
			case <-ctx.Done():
				cancel()
			}
		}()

		builder := controllercmd.NewController("openshift-insights-operator", operator.Run, clock.RealClock{}).
			WithKubeConfigFile(cmd.Flags().Lookup("kubeconfig").Value.String(), nil).
			WithLeaderElection(operatorConfig.LeaderElection, "", "openshift-insights-operator-lock").
			WithServer(operatorConfig.ServingInfo, operatorConfig.Authentication, operatorConfig.Authorization).
			WithRestartOnChange(exitOnChangeReactorCh, startingFileContent, observedFiles...)
		if err := builder.Run(ctx2, unstructured); err != nil {
			klog.Error(err)
		}
	}
}

// Starts a single gather, main responsibility is loading in the necessary configs.
func runGather(operator *controller.GatherJob, cfg *controllercmd.ControllerCommandConfig) func(cmd *cobra.Command, _ []string) {
	return func(cmd *cobra.Command, _ []string) {
		clientConfig, protoConfig := createClientConfig(cmd, operator, cfg)

		// Before the start of gathering we need to check if featureGates are enabled
		ctx, cancel := context.WithTimeout(context.Background(), operator.Interval)
		checkFeatureGates(ctx, clientConfig, operator)

		// Run gatherer
		if err := operator.Gather(ctx, clientConfig, protoConfig); err != nil {
			klog.Error(err)
		}

		cancel()
		os.Exit(0)
	}
}

// Starts a single gather, main responsibility is loading in the necessary configs.
func runGatherAndUpload(
	operator *controller.GatherJob, cfg *controllercmd.ControllerCommandConfig,
) func(cmd *cobra.Command, _ []string) {
	return func(cmd *cobra.Command, _ []string) {
		clientConfig, protoConfig := createClientConfig(cmd, operator, cfg)

		// Before the start of gathering we need to check if featureGates are enabled
		ctx, cancel := context.WithTimeout(context.Background(), operator.Interval)
		checkFeatureGates(ctx, clientConfig, operator)

		// Run gatherer
		if err := operator.GatherAndUpload(clientConfig, protoConfig); err != nil {
			klog.Exit(err)
		}

		cancel()
		os.Exit(0)
	}
}

func checkFeatureGates(ctx context.Context, clientConfig *rest.Config, operator *controller.GatherJob) {
	configClientSet, err := configv1client.NewForConfig(clientConfig)
	if err != nil {
		klog.Exitf("error: building client: %v", err)
	}
	configInformers := configv1informers.NewSharedInformerFactory(configClientSet, 10*time.Minute)

	desiredVersion := missingVersion
	if envVersion, exists := os.LookupEnv("RELEASE_VERSION"); exists {
		desiredVersion = envVersion
	}

	// By default, this will exit(0) the process if the featuregates ever change to a different set of values.
	featureGateAccessor := featuregates.NewFeatureGateAccess(
		desiredVersion, missingVersion,
		configInformers.Config().V1().ClusterVersions(), configInformers.Config().V1().FeatureGates(),
		events.NewLoggingEventRecorder("insights-gather", clock.RealClock{}),
	)
	go featureGateAccessor.Run(ctx)
	go configInformers.Start(ctx.Done())

	select {
	case <-featureGateAccessor.InitialFeatureGatesObserved():
		featureGates, _ := featureGateAccessor.CurrentFeatureGates()
		klog.Infof("FeatureGates initialized: knownFeatureGates=%v", featureGates.KnownFeatures())
	case <-time.After(1 * time.Minute):
		klog.Errorf("timed out waiting for FeatureGate detection")
		klog.Exit(fmt.Errorf("timed out waiting for FeatureGate detection"))
	}

	featureGates, err := featureGateAccessor.CurrentFeatureGates()
	if err != nil {
		klog.Exit(err)
	}

	operator.InsightsConfigAPIEnabled = featureGates.Enabled(features.FeatureGateInsightsConfigAPI)
	operator.RuntimeExtractorEnabled = featureGates.Enabled(features.FeatureGateInsightsRuntimeExtractor)
}

func createClientConfig(
	cmd *cobra.Command, operator *controller.GatherJob, cfg *controllercmd.ControllerCommandConfig,
) (clientConfig, protoConfig *rest.Config) {
	if configArg := cmd.Flags().Lookup("config").Value.String(); len(configArg) == 0 {
		klog.Exit("error: --config is required")
	}

	unstructured, _, _, err := cfg.Config()
	if err != nil {
		klog.Exit(err)
	}

	cont, err := config.LoadConfig(operator.Controller, unstructured.Object, config.ToDisconnectedController)
	if err != nil {
		klog.Exit(err)
	}
	operator.Controller = cont

	var clientCfg *rest.Config
	if kubeConfigPath := cmd.Flags().Lookup("kubeconfig").Value.String(); len(kubeConfigPath) > 0 {
		kubeConfigBytes, err := os.ReadFile(kubeConfigPath) //nolint: govet
		if err != nil {
			klog.Exit(err)
		}

		kubeConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
		if err != nil {
			klog.Exit(err)
		}

		clientCfg, err = kubeConfig.ClientConfig()
		if err != nil {
			klog.Exit(err)
		}
	} else {
		clientCfg, err = rest.InClusterConfig()
		if err != nil {
			klog.Exit(err)
		}
	}

	protoCfg := rest.CopyConfig(clientCfg)
	protoCfg.AcceptContentTypes = pbAcceptContentTypes
	protoCfg.ContentType = pbContentType

	return clientCfg, protoCfg
}
