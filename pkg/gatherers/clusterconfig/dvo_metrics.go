package clusterconfig

import (
	"context"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

func (g *Gatherer) GatherDVOMetrics(ctx context.Context) ([]record.Record, []error) {
	apiURL, err := url.Parse("http://deployment-validation-operator-metrics.deployment-validation-operator.svc:8383")
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
	data, err := metricsClient.Get().AbsPath("metrics").
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve most recent metrics: %v", err)
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/dvo_metrics", Item: marshal.RawByte(data)},
	}

	return records, nil
}
