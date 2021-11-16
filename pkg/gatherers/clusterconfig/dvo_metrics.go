package clusterconfig

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

const (
	dvoNamespace = "deployment-validation-operator"
	// dvo_metrics_endpoint = "http://deployment-validation-operator-metrics.deployment-validation-operator.svc:8383"
)

var (
	dvoMetricsServiceNameRegex = regexp.MustCompile(`\bmetrics\b`)
	dvoMetricsPrefix           = []byte("deployment_validation_operator_")
)

func (g *Gatherer) GatherDVOMetrics(ctx context.Context) ([]record.Record, []error) {
	// apiURL, err := url.Parse(dvo_metrics_endpoint)
	// if err != nil {
	// 	return nil, []error{err}
	// }
	// metricsRESTClient, err := rest.NewRESTClient(apiURL, "/", rest.ClientContentConfig{}, g.gatherKubeConfig.RateLimiter, http.DefaultClient)
	// if err != nil {
	// 	klog.Warningf("Unable to load metrics client, no metrics will be collected: %v", err)
	// 	return nil, nil
	// }
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherDVOMetrics(ctx, gatherKubeClient.CoreV1(), g.gatherKubeConfig.RateLimiter)
}

func gatherDVOMetrics(ctx context.Context, coreClient corev1client.CoreV1Interface, rateLimiter flowcontrol.RateLimiter) ([]record.Record, []error) {
	serviceList, err := coreClient.Services(dvoNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	allDVOMetricsLines := []byte{}

	for _, service := range serviceList.Items {
		if !dvoMetricsServiceNameRegex.MatchString(service.Name) {
			continue
		}
		for _, port := range service.Spec.Ports {
			apiURL := url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s.%s.svc:%d", service.Name, dvoNamespace, port.Port),
			}
			metricsRESTClient, err := rest.NewRESTClient(&apiURL, "/", rest.ClientContentConfig{}, rateLimiter, http.DefaultClient)
			if err != nil {
				klog.Warningf("Unable to load metrics client, no metrics will be collected: %v", err)
				return nil, nil
			}

			dataReader, err := metricsRESTClient.Get().AbsPath("metrics").Stream(ctx)
			defer func() {
				if err := dataReader.Close(); err != nil {
					klog.Errorf("Unable to close metrics stream: %v", err)
				}
			}()
			if err != nil {
				klog.Errorf("Unable to retrieve most recent metrics: %v", err)
				return nil, []error{err}
			}

			prefixedLines, err := utils.ReadAllLinesWithPrefix(dataReader, dvoMetricsPrefix)
			if err != io.EOF {
				klog.Errorf("Unable to read metrics lines with DVO prefix: %v", err)
				return nil, []error{err}
			}

			allDVOMetricsLines = append(allDVOMetricsLines, prefixedLines...)
		}
	}

	records := []record.Record{
		{Name: "config/dvo_metrics_filtered", Item: marshal.RawByte(allDVOMetricsLines)},
	}

	return records, nil
}
