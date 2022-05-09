package start

import (
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/serviceability"

	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller"
)

const serviceCACertPath = "/var/run/configmaps/service-ca-bundle/service-ca.crt"

// NewOperator create the commad for running the Insights Operator.
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
				ClusterTransferInterval: 24 * time.Hour,
			},
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), operator.Run)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the operator",
		Run:   runOperator(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommand().Flags())

	return cmd
}

// NewGather create the commad for running the a single gather.
func NewGather() *cobra.Command {
	operator := &controller.GatherJob{
		Controller: config.Controller{
			ConditionalGathererEndpoint: "https://console.redhat.com/api/gathering/gathering_rules",
			StoragePath:                 "/var/lib/insights-operator",
			Interval:                    30 * time.Minute,
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), nil)
	cmd := &cobra.Command{
		Use:   "gather",
		Short: "Does a single gather, without uploading it",
		Run:   runGather(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommand().Flags())

	return cmd
}

// Starts a single gather, main responsibility is loading in the necessary configs.
func runGather(operator *controller.GatherJob, cfg *controllercmd.ControllerCommandConfig) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
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

		var clientConfig *rest.Config
		if kubeConfigPath := cmd.Flags().Lookup("kubeconfig").Value.String(); len(kubeConfigPath) > 0 {
			kubeConfigBytes, err := os.ReadFile(kubeConfigPath) //nolint: govet
			if err != nil {
				klog.Exit(err)
			}
			kubeConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
			if err != nil {
				klog.Exit(err)
			}
			clientConfig, err = kubeConfig.ClientConfig()
			if err != nil {
				klog.Exit(err)
			}
		} else {
			clientConfig, err = rest.InClusterConfig()
			if err != nil {
				klog.Exit(err)
			}
		}
		protoConfig := rest.CopyConfig(clientConfig)
		protoConfig.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
		protoConfig.ContentType = "application/vnd.kubernetes.protobuf"

		ctx, cancel := context.WithTimeout(context.Background(), operator.Interval)
		err = operator.Gather(ctx, clientConfig, protoConfig)
		if err != nil {
			klog.Error(err)
		}
		cancel()
		os.Exit(0)
	}
}

// Boilerplate for running an operator and handling command line arguments.
func runOperator(operator *controller.Operator, cfg *controllercmd.ControllerCommandConfig) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		// boiler plate for the "normal" command
		rand.Seed(time.Now().UTC().UnixNano())
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
			klog.V(4).Infof("Unable to read service ca bundle: %v", err)
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

		builder := controllercmd.NewController("openshift-insights-operator", operator.Run).
			WithKubeConfigFile(cmd.Flags().Lookup("kubeconfig").Value.String(), nil).
			WithLeaderElection(operatorConfig.LeaderElection, "", "openshift-insights-operator-lock").
			WithServer(operatorConfig.ServingInfo, operatorConfig.Authentication, operatorConfig.Authorization).
			WithRestartOnChange(exitOnChangeReactorCh, startingFileContent, observedFiles...)
		if err := builder.Run(ctx2, unstructured); err != nil {
			klog.Error(err)
		}
	}
}
