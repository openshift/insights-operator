package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/eparis/urlhash"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

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

func getNamespace() (string, error) {
	nsBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}
	return string(nsBytes), nil
}

func (s *Support) setupURLHash(kubeClient *kubernetes.Clientset) error {
	namespace, err := getNamespace()
	if err != nil || namespace == "" {
		klog.Warning("Unable to determine namespace. IP addresses will not be anonymized")
		return nil
	}
	saltSecret, err := kubeClient.CoreV1().Secrets(namespace).Get("urlsalt", metav1.GetOptions{})
	if err != nil {
		//if err != ENOTEXIST Bail
		saltSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "urlsalt",
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"AnonymizeURLSalt": urlhash.GetNewSalt(10),
			},
		}
		saltSecret, err = kubeClient.CoreV1().Secrets(namespace).Create(saltSecret)
		if err != nil {
			return err
		}
	}

	saltByte, exist := saltSecret.Data["AnonymizeURLSalt"]
	if !exist {
		return fmt.Errorf("unable to find AnonymizeURLSalt in secret: openshift-insights-operator/URLSalt")
	}
	urlhash.SetSalt(string(saltByte))
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

	if err := s.setupURLHash(kubeClient); err != nil {
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
