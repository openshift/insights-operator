package clusterconfig

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

const dvo_metrics_endpoint = "http://deployment-validation-operator-metrics.deployment-validation-operator.svc:8383"

var dvo_metrics_prefix = []byte("deployment_validation_operator_")

func (g *Gatherer) GatherDVOMetrics(ctx context.Context) ([]record.Record, []error) {
	apiURL, err := url.Parse(dvo_metrics_endpoint)
	if err != nil {
		return nil, []error{err}
	}
	metricsRESTClient, err := rest.NewRESTClient(apiURL, "/", rest.ClientContentConfig{}, g.gatherKubeConfig.RateLimiter, http.DefaultClient)
	if err != nil {
		klog.Warningf("Unable to load metrics client, no metrics will be collected: %v", err)
		return nil, nil
	}

	return gatherDVOMetrics(ctx, metricsRESTClient)
}

func gatherDVOMetrics(ctx context.Context, metricsClient rest.Interface) ([]record.Record, []error) {
	dataReader, err := metricsClient.Get().AbsPath("metrics").Stream(ctx)
	defer func() {
		if err := dataReader.Close(); err != nil {
			klog.Errorf("Unable to close metrics stream: %v", err)
		}
	}()
	if err != nil {
		klog.Errorf("Unable to retrieve most recent metrics: %v", err)
		return nil, []error{err}
	}

	prefixedLines, err := utils.ReadAllLinesWithPrefix(dataReader, dvo_metrics_prefix)
	if err != io.EOF {
		klog.Errorf("Unable to read metrics lines with DVO prefix: %v", err)
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/dvo_metrics_filtered", Item: marshal.RawByte(prefixedLines)},
	}

	return records, nil
}
