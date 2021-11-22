package clusterconfig

import (
	"bytes"
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
)

var (
	// Only services with the word "metrics" in their name should be considered.
	dvoMetricsServiceNameRegex = regexp.MustCompile(`\bmetrics\b`)
	// Only metrics with the DVO prefix should be gathered.
	dvoMetricsPrefix = []byte("deployment_validation_operator_")
)

// GatherDVOMetrics collects metrics from the Deployment Validation Operator's
// metrics service. The metrics are fetched via the /metrics endpoint and
// filtered to only include those with a deployment_validation_operator_ prefix.
//
// * Location in archive: config/dvo_metrics
// * See: docs/insights-archive-sample/config/dvo_metrics
// * Id in config: dvo_metrics
// * Since version:
//   - 4.10
func (g *Gatherer) GatherDVOMetrics(ctx context.Context) ([]record.Record, []error) {
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

	nonFatalErrors := []error{}
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

			prefixedLines, err := gatherDVOMetricsFromEndpoint(ctx, apiURL, rateLimiter)
			if err != nil {
				// Log errors as warnings and don't fail the entire gatherer.
				// It is possible that this service is not really the correct one
				// and a different service may return the metrics we are looking for.
				klog.Warningf("Unable to read metrics from endpoint %q: %v", apiURL, err)
				nonFatalErrors = append(nonFatalErrors, err)
				continue
			}

			// Make sure the metrics are terminated by a final line separator or completely empty.
			if len(prefixedLines) > 0 && !bytes.HasSuffix(prefixedLines, utils.MetricsLineSep) {
				prefixedLines = append(prefixedLines, utils.MetricsLineSep...)
			}

			allDVOMetricsLines = append(allDVOMetricsLines, []byte(fmt.Sprintf("# %v\n", apiURL))...)
			allDVOMetricsLines = append(allDVOMetricsLines, prefixedLines...)
		}
	}

	records := []record.Record{
		{Name: "config/dvo_metrics", Item: marshal.RawByte(allDVOMetricsLines)},
	}

	return records, nonFatalErrors
}

func gatherDVOMetricsFromEndpoint(ctx context.Context, apiURL url.URL, rateLimiter flowcontrol.RateLimiter) ([]byte, error) {
	metricsRESTClient, err := rest.NewRESTClient(&apiURL, "/", rest.ClientContentConfig{}, rateLimiter, http.DefaultClient)
	if err != nil {
		klog.Warningf("Unable to load metrics client, no metrics will be collected: %v", err)
		return nil, err
	}

	dataReader, err := metricsRESTClient.Get().AbsPath("metrics").Stream(ctx)
	if err != nil {
		klog.Warningf("Unable to retrieve most recent metrics: %v", err)
		return nil, err
	}
	defer func() {
		if err := dataReader.Close(); err != nil {
			klog.Errorf("Unable to close metrics stream: %v", err)
		}
	}()

	prefixedLines, err := utils.ReadAllLinesWithPrefix(dataReader, dvoMetricsPrefix)
	if err != io.EOF {
		klog.Warningf("Unable to read metrics lines with DVO prefix: %v", err)
		return prefixedLines, err
	}
	return prefixedLines, nil
}
