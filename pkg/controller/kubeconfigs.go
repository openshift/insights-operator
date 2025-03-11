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

	token := strings.TrimSpace(os.Getenv(insecurePrometheusTokenEnvVariable))

	// the metrics client will connect to prometheus and scrape a small set of metrics
	metricsGatherKubeConfig = createGatherConfig(kubeConfig, metricHost, token)
	// the alerts client will connect to alert manager and collect a set of silences
	alertsGatherKubeConfig = createGatherConfig(kubeConfig, alertManagerHost, token)

	return gatherProtoKubeConfig, gatherKubeConfig, metricsGatherKubeConfig, alertsGatherKubeConfig
}

func createGatherConfig(kubeConfig *rest.Config, configHost, token string) *rest.Config {
	gatherConfig := rest.CopyConfig(kubeConfig)

	gatherConfig.CAFile = metricCAFile
	gatherConfig.NegotiatedSerializer = scheme.Codecs
	gatherConfig.GroupVersion = &schema.GroupVersion{}
	gatherConfig.APIPath = "/"
	gatherConfig.Host = configHost

	if len(token) > 0 {
		klog.Infof("using insecure prometheus token")
		gatherConfig.Insecure = true
		gatherConfig.BearerToken = token
		// by default CAFile is /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
		gatherConfig.CAFile = ""
		gatherConfig.CAData = []byte{}
	}

	return gatherConfig
}
