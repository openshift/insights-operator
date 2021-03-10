package controller

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gather/clusterconfig"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
)

type GatherJob struct {
	config.Controller
}

func (d *GatherJob) Gather(ctx context.Context, kubeConfig *rest.Config, protoKubeConfig *rest.Config) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())
	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return err
	}
	// these are gathering clients
	gatherProtoKubeConfig := rest.CopyConfig(protoKubeConfig)
	if len(d.Impersonate) > 0 {
		gatherProtoKubeConfig.Impersonate.UserName = d.Impersonate
	}
	gatherKubeConfig := rest.CopyConfig(kubeConfig)
	if len(d.Impersonate) > 0 {
		gatherKubeConfig.Impersonate.UserName = d.Impersonate
	}

	// the metrics client will connect to prometheus and scrape a small set of metrics
	// TODO: the oauth-proxy and delegating authorizer do not support Impersonate-User,
	//   so we do not impersonate gather
	metricsGatherKubeConfig := rest.CopyConfig(kubeConfig)
	metricsGatherKubeConfig.CAFile = "/var/run/configmaps/service-ca-bundle/service-ca.crt"
	metricsGatherKubeConfig.NegotiatedSerializer = scheme.Codecs
	metricsGatherKubeConfig.GroupVersion = &schema.GroupVersion{}
	metricsGatherKubeConfig.APIPath = "/"
	metricsGatherKubeConfig.Host = "https://prometheus-k8s.openshift-monitoring.svc:9091"

	// ensure the insight snapshot directory exists
	if _, err := os.Stat(d.StoragePath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(d.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(d.Controller, kubeClient)

	// the recorder periodically flushes any recorded data to disk as tar.gz files
	// in s.StoragePath, and also prunes files above a certain age
	recdriver := diskrecorder.New(d.StoragePath)
	recorder := recorder.New(recdriver, d.Interval)

	// the gatherers periodically check the state of the cluster and report any
	// config to the recorder
	clusterConfigGatherer := clusterconfig.New(gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig)
	err = clusterConfigGatherer.Gather(ctx, configObserver.Config().Gather, recorder)
	if err != nil {
		return err
	}
	recorder.Flush()

	klog.Warning("stopped")
	return nil
}
