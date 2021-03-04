package start

import (
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/serviceability"
	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/version"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controller"
)

const serviceCACertPath = "/var/run/configmaps/service-ca-bundle/service-ca.crt"

type operatorController interface {
	Run(ctx context.Context, controller *controllercmd.ControllerContext) error
}

func NewOperator() *cobra.Command {
	operator := &controller.Support{
		Controller: config.Controller{
			StoragePath:          "/var/lib/insights-operator",
			Interval:             10 * time.Minute,
			Endpoint:             "https://cloud.redhat.com/api/ingress/v1/upload",
			ReportEndpoint:       "https://cloud.redhat.com/api/insights-results-aggregator/v1/clusters/%s/report",
			ReportPullingDelay:   60 * time.Second,
			ReportMinRetryTime:   10 * time.Second,
			ReportPullingTimeout: 30 * time.Minute,
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), operator.Run)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the operator",
		Run:   createRunnable(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommand().Flags())

	return cmd
}

func NewDisconnectedOperator() *cobra.Command {
	operator := &controller.Disconnected{
		Controller: config.Controller{
			StoragePath: "/var/lib/insights-operator",
			Interval:    10 * time.Minute,
		},
	}
	cfg := controllercmd.NewControllerCommandConfig("openshift-insights-operator", version.Get(), operator.Run)
	cmd := &cobra.Command{
		Use:   "start-disconnected",
		Short: "Start the operator in disconnected mode",
		Run:   createRunnable(operator, cfg),
	}
	cmd.Flags().AddFlagSet(cfg.NewCommand().Flags())

	return cmd
}

func createRunnable(operator operatorController, cfg *controllercmd.ControllerCommandConfig) func(cmd *cobra.Command, args []string){
	return func(cmd *cobra.Command, args []string) {
		// boiler plate for the "normal" command
		rand.Seed(time.Now().UTC().UnixNano())
		defer serviceability.BehaviorOnPanic(os.Getenv("OPENSHIFT_ON_PANIC"), version.Get())()
		defer serviceability.Profile(os.Getenv("OPENSHIFT_PROFILE")).Stop()
		serviceability.StartProfiler()

		if config := cmd.Flags().Lookup("config").Value.String(); len(config) == 0 {
			klog.Fatalf("error: --config is required")
		}

		unstructured, config, configBytes, err := cfg.Config()
		if err != nil {
			klog.Fatal(err)
		}

		startingFileContent, observedFiles, err := cfg.AddDefaultRotationToConfig(config, configBytes)
		if err != nil {
			klog.Fatal(err)
		}

		// if the service CA is rotated, we want to restart
		if data, err := ioutil.ReadFile(serviceCACertPath); err == nil {
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
			WithLeaderElection(config.LeaderElection, "", "openshift-insights-operator-lock").
			WithServer(config.ServingInfo, config.Authentication, config.Authorization).
			WithRestartOnChange(exitOnChangeReactorCh, startingFileContent, observedFiles...)

		if err := builder.Run(ctx2, unstructured); err != nil {
			klog.Fatal(err)
		}
	}
}
