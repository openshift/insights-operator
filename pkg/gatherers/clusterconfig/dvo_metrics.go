package clusterconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// GatherDVOMetrics Collects metrics from the Deployment Validation Operator's
// metrics service. The metrics are fetched via the /metrics endpoint and
// filtered to only include those with a `deployment_validation_operator_` prefix.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/dvo_metrics
//
// ### Location in archive
// - `config/dvo_metrics`
//
// ### Config ID
// `clusterconfig/dvo_metrics`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherDVOMetrics(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherDVOMetrics(ctx, gatherKubeClient.CoreV1(), g.gatherKubeConfig.RateLimiter)
}

func gatherDVOMetrics(
	ctx context.Context,
	coreClient corev1client.CoreV1Interface,
	rateLimiter flowcontrol.RateLimiter,
) ([]record.Record, []error) {
	serviceList, err := coreClient.Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "name=deployment-validation-operator",
	})
	if err != nil {
		return nil, []error{err}
	}

	nonFatalErrors := []error{}
	allDVOMetricsLines := []byte{}
	for svcIdx := range serviceList.Items {
		// Use pointer to make gocritic happy and avoid copying the whole Service struct.
		service := &serviceList.Items[svcIdx]
		for _, port := range service.Spec.Ports {
			apiURL := url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s.%s.svc:%d", service.Name, service.Namespace, port.Port),
			}

			prefixedLines, err := gatherDVOMetricsFromEndpoint(ctx, &apiURL, rateLimiter)
			if err != nil {
				// Log errors as warnings and don't fail the entire gatherer.
				// It is possible that this service is not really the correct one
				// and a different service may return the metrics we are looking for.
				klog.Warningf("Unable to read metrics from endpoint %q: %v", apiURL.String(), err)
				nonFatalErrors = append(nonFatalErrors, err)
				continue
			}

			// Make sure the metrics are terminated by a final line separator or completely empty.
			if len(prefixedLines) > 0 && !bytes.HasSuffix(prefixedLines, utils.MetricsLineSep) {
				prefixedLines = append(prefixedLines, utils.MetricsLineSep...)
			}

			metricsHeading := fmt.Sprintf("# %s\n", apiURL.String())
			allDVOMetricsLines = append(allDVOMetricsLines, []byte(metricsHeading)...)
			allDVOMetricsLines = append(allDVOMetricsLines, prefixedLines...)
		}
	}

	// If there are no DVO metrics (or no DVO metrics service), don't create a record at all.
	if len(allDVOMetricsLines) == 0 {
		klog.Warning("No DVO metrics gathered")
		return nil, nonFatalErrors
	}

	return []record.Record{
		{Name: "config/dvo_metrics", Item: marshal.RawByte(allDVOMetricsLines)},
	}, nonFatalErrors
}

func gatherDVOMetricsFromEndpoint(
	ctx context.Context,
	apiURL *url.URL,
	rateLimiter flowcontrol.RateLimiter,
) ([]byte, error) {
	metricsRESTClient, err := rest.NewRESTClient(
		apiURL,
		"/",
		rest.ClientContentConfig{},
		rateLimiter,
		http.DefaultClient,
	)
	if err != nil {
		klog.Errorf("Unable to load metrics client, no metrics will be collected: %v", err)
		return nil, err
	}

	err = metricsServiceUp(ctx, metricsRESTClient, apiURL)
	if err != nil {
		return nil, err
	}

	dataReader, err := metricsRESTClient.Get().AbsPath("metrics").Stream(ctx)
	if err != nil {
		klog.Errorf("Failed to stream data from the DVO service: %v", err)
		return nil, err
	}

	defer func() {
		// The error variable must have a more unique name to satisfy govet.
		if closeErr := dataReader.Close(); closeErr != nil {
			klog.Errorf("Unable to close metrics stream: %v", closeErr)
		}
	}()

	// only metrics with the DVO prefix should be gathered.
	dvoMetricsPrefix := []byte("deployment_validation_operator_")
	// precompile regex rules
	regexString := `(?m)(,?%s=[^\,\}]*")`
	filterProps := []*regexp.Regexp{
		regexp.MustCompile(fmt.Sprintf(regexString, "name")),
		regexp.MustCompile(fmt.Sprintf(regexString, "namespace")),
	}
	prefixedLines, err := utils.ReadAllLinesWithPrefix(dataReader, dvoMetricsPrefix, func(b []byte) []byte {
		for _, re := range filterProps {
			str := re.ReplaceAllString(string(b), "")
			b = []byte(str)
		}
		return b
	})
	if err != io.EOF {
		klog.Warningf("Unable to read metrics lines with DVO prefix: %v", err)
		return prefixedLines, err
	}

	return prefixedLines, nil
}

// metricsServiceUp queries the DVO metrics service with poll in case of an error. Polling
// timeout is 5 seconds and interval is 1 second.
func metricsServiceUp(ctx context.Context, client *rest.RESTClient, apiURL *url.URL) error {
	timeout := 5 * time.Second
	dvoMetricsURL := fmt.Sprintf("http://%s/metrics", apiURL.Host)
	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (done bool, err error) {
		resp, err := client.Client.Head(dvoMetricsURL) //nolint: noctx
		if err != nil || resp.StatusCode != 200 {
			klog.Warning("Failed to read the DVO metrics. Trying again.")
			return false, nil
		}
		defer resp.Body.Close()
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("DVO metrics service was not available within the %s timeout: %v", timeout, err)
	}
	return nil
}
