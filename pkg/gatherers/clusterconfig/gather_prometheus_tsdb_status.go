package clusterconfig

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// GatherPrometheusTSDBStatus Collects Prometheus TSDB status.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/tsdb.json
//
// ### Location in archive
// - `config/tsdb.json`
//
// ### Config ID
// `clusterconfig/tsdb_status`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherPrometheusTSDBStatus(ctx context.Context) ([]record.Record, []error) {
	metricsRESTClient, err := rest.RESTClientFor(g.metricsGatherKubeConfig)
	if err != nil {
		klog.Warningf("Unable to load metrics client, tsdb status cannot be collected: %v", err)
		return nil, nil
	}

	return gatherPrometheusTSDBStatus(ctx, metricsRESTClient)
}

func gatherPrometheusTSDBStatus(ctx context.Context, metricsClient rest.Interface) ([]record.Record, []error) {
	data, err := metricsClient.Get().AbsPath("api/v1/status/tsdb").
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to tsdb status: %v", err)
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/tsdb.json", Item: marshal.RawByte(data)},
	}

	return records, nil
}
