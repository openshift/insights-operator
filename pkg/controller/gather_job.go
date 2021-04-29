package controller

import (
	"context"
	"fmt"
	"os"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/version"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
)

// Object responsible for controlling a non-periodic Gather execution
type GatherJob struct {
	config.Controller
}

// Runs a single gather and stores the generated archive, without uploading it.
// 1. Creates the necessary configs/clients
// 2. Creates the configobserver to get more configs
// 3. Initiates the recorder
// 4. Executes a Gather
// 5. Flushes the results
func (d *GatherJob) Gather(ctx context.Context, kubeConfig, protoKubeConfig *rest.Config) error {
	klog.Infof("Starting insights-operator %s", version.Get().String())
	// these are operator clients
	kubeClient, err := kubernetes.NewForConfig(protoKubeConfig)
	if err != nil {
		return err
	}
	// these are gathering configs
	gatherProtoKubeConfig := rest.CopyConfig(protoKubeConfig)
	if len(d.Impersonate) > 0 {
		gatherProtoKubeConfig.Impersonate.UserName = d.Impersonate
	}
	gatherKubeConfig := rest.CopyConfig(kubeConfig)
	if len(d.Impersonate) > 0 {
		gatherKubeConfig.Impersonate.UserName = d.Impersonate
	}

	// the metrics client will connect to prometheus and scrape a small set of metrics
	metricsGatherKubeConfig := rest.CopyConfig(kubeConfig)
	metricsGatherKubeConfig.CAFile = metricCAFile
	metricsGatherKubeConfig.NegotiatedSerializer = scheme.Codecs
	metricsGatherKubeConfig.GroupVersion = &schema.GroupVersion{}
	metricsGatherKubeConfig.APIPath = "/"
	metricsGatherKubeConfig.Host = metricHost

	// ensure the insight snapshot directory exists
	if _, err = os.Stat(d.StoragePath); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(d.StoragePath, 0777); err != nil {
			return fmt.Errorf("can't create --path: %v", err)
		}
	}

	// configobserver synthesizes all config into the status reporter controller
	configObserver := configobserver.New(d.Controller, kubeClient)

	var anonymizer *anonymization.Anonymizer
	if anonymization.IsObfuscationEnabled(configObserver) {
		configClient, err := configv1client.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}
		// anonymizer is responsible for anonymizing sensitive data, it can be configured to disable specific anonymization
		anonymizer, err = anonymization.NewAnonymizerFromConfigClient(ctx, kubeClient, configClient)
		if err != nil {
			return err
		}
	}

	// the recorder stores the collected data and we flush at the end.
	recdriver := diskrecorder.New(d.StoragePath)
	rec := recorder.New(recdriver, d.Interval, anonymizer)
	defer rec.Flush()

	gatherers := gather.CreateAllGatherers(
		gatherKubeConfig, gatherProtoKubeConfig, metricsGatherKubeConfig, anonymizer,
	)

	var allFunctionReports []gather.GathererFunctionReport
	for _, gatherer := range gatherers {
		functionReports, err := gather.CollectAndRecordGatherer(ctx, gatherer, rec, configObserver)
		if err != nil {
			return err
		}
		allFunctionReports = append(allFunctionReports, functionReports...)
	}

	return gather.RecordArchiveMetadata(allFunctionReports, rec, anonymizer)
}
