package controller

import (
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

func prepareGatherConfigs(protoKubeConfig, kubeConfig *rest.Config, impersonate string) (
	gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig *rest.Config,
) {
	// these are gathering configs
	gatherProtoKubeConfig = rest.CopyConfig(protoKubeConfig)
	if len(impersonate) > 0 {
		gatherProtoKubeConfig.Impersonate.UserName = impersonate
	}
	gatherKubeConfig = rest.CopyConfig(kubeConfig)
	if len(impersonate) > 0 {
		gatherKubeConfig.Impersonate.UserName = impersonate
	}

	// the metrics client will connect to prometheus and scrape a small set of metrics
	metricsGatherKubeConfig = rest.CopyConfig(kubeConfig)
	metricsGatherKubeConfig.CAFile = metricCAFile
	metricsGatherKubeConfig.NegotiatedSerializer = scheme.Codecs
	metricsGatherKubeConfig.GroupVersion = &schema.GroupVersion{}
	metricsGatherKubeConfig.APIPath = "/"
	metricsGatherKubeConfig.Host = metricHost

	// the alerts client will connect to alert manager and collect a set of silences
	alertsGatherKubeConfig = rest.CopyConfig(kubeConfig)
	alertsGatherKubeConfig.CAFile = metricCAFile
	alertsGatherKubeConfig.NegotiatedSerializer = scheme.Codecs
	alertsGatherKubeConfig.GroupVersion = &schema.GroupVersion{}
	alertsGatherKubeConfig.APIPath = "/"
	alertsGatherKubeConfig.Host = alertManagerHost

	if token := strings.TrimSpace(os.Getenv(insecurePrometheusTokenEnvVariable)); len(token) > 0 {
		klog.Infof("using insecure prometheus token")
		metricsGatherKubeConfig.Insecure = true
		metricsGatherKubeConfig.BearerToken = token
		// by default CAFile is /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
		metricsGatherKubeConfig.CAFile = ""
		metricsGatherKubeConfig.CAData = []byte{}

		alertsGatherKubeConfig.Insecure = true
		alertsGatherKubeConfig.BearerToken = token
		// by default CAFile is /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
		alertsGatherKubeConfig.CAFile = ""
		alertsGatherKubeConfig.CAData = []byte{}
	}

	return gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig
}
