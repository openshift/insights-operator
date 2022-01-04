package clusterconfig

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// GatherMostRecentMetrics gathers cluster Federated Monitoring metrics.
//
// The GET REST query to URL /federate
// Gathered metrics:
// 	 virt_platform
//   etcd_object_counts
//   cluster_installer
//   vsphere_node_hw_version_total
//   namespace CPU and memory usage
//   followed by at most 1000 lines of ALERTS metric
//
// * Location in archive: config/metrics
// * See: docs/insights-archive-sample/config/metrics
// * Id in config: metrics
// * Since version:
//   - "etcd_object_counts": 4.3+
//   - "cluster_installer": 4.3+
//   - "ALERTS": 4.3+
//   - "namespace:container_cpu_usage_seconds_total:sum_rate": 4.5+
//   - "namespace:container_memory_usage_bytes:sum": 4.5+
//   - "virt_platform metric": 4.6.34+, 4.7.16+, 4.8+
//   - "vsphere_node_hw_version_total": 4.7.11+, 4.8+
func (g *Gatherer) GatherMostRecentMetrics(ctx context.Context) ([]record.Record, []error) {
	metricsRESTClient, err := rest.RESTClientFor(g.metricsGatherKubeConfig)
	if err != nil {
		klog.Warningf("Unable to load metrics client, no metrics will be collected: %v", err)
		return nil, nil
	}

	return gatherMostRecentMetrics(ctx, metricsRESTClient)
}

func gatherMostRecentMetrics(ctx context.Context, metricsClient rest.Interface) ([]record.Record, []error) {
	data, err := metricsClient.Get().AbsPath("federate").
		Param("match[]", "etcd_object_counts").
		Param("match[]", "cluster_installer").
		Param("match[]", "namespace:container_cpu_usage_seconds_total:sum_rate").
		Param("match[]", "namespace:container_memory_usage_bytes:sum").
		Param("match[]", "vsphere_node_hw_version_total").
		Param("match[]", "virt_platform").
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve most recent metrics: %v", err)
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/metrics", Item: marshal.RawByte(data)},
	}

	return records, nil
}
