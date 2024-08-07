package clusterconfig

import (
	"context"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

// GatherMostRecentMetrics Collects cluster Federated Monitoring metrics.
//
// The GET REST query to URL /federate
// Gathered metrics:
//   - `virt_platform`
//   - `cluster_installer`
//   - `vsphere_node_hw_version_total`
//   - namespace CPU and memory usage
//   - `console_helm_installs_total`
//   - `console_helm_upgrades_total`
//   - `console_helm_uninstalls_total`
//   - `etcd_server_slow_apply_total`
//   - `etcd_server_slow_read_indexes_total`
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/metrics
//
// ### Location in archive
// - `config/metrics`
//
// ### Config ID
// `clusterconfig/metrics`
//
// ### Released version
// - 4.3.0
//
// ### Backported versions
// None
//
// ### Changes
// - `etcd_object_counts` introduced in version 4.3+ and removed in 4.12.0
// - `cluster_installer` introduced in version 4.3+
// - `ALERTS` introduced in version 4.3+
// - `namespace:container_cpu_usage_seconds_total:sum_rate` introduced in version 4.5+ and changed to `namespace:container_cpu_usage:sum` in 4.16.0+
// - `namespace:container_memory_usage_bytes:sum` introduced in version 4.5+
// - `virt_platform metric` introduced in version 4.8+ and backported to 4.6.34+, 4.7.16+ versions
// - `vsphere_node_hw_version_total` introduced in version 4.8+ and backported to 4.7.11+ version
// - `console_helm_installs_total` introduced in version 4.11+
// - `console_helm_upgrades_total` introduced in version 4.12+
// - `console_helm_uninstalls_total` introduced in version 4.12+
// - `openshift_apps_deploymentconfigs_strategy_total` introduced in version 4.13+ and backported to 4.12.5+ version
// - `etcd_server_slow_apply_total` introduced in version 4.16+
// - `etcd_server_slow_read_indexes_total` introduced in version 4.16+
// - `haproxy_exporter_server_threshold` introduced in version 4.17+
// - `ALERTS` removed in version 4.17+
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
		Param("match[]", "cluster_installer").
		Param("match[]", "namespace:container_cpu_usage:sum").
		Param("match[]", "namespace:container_memory_usage_bytes:sum").
		Param("match[]", "vsphere_node_hw_version_total").
		Param("match[]", "virt_platform").
		Param("match[]", "console_helm_installs_total").
		Param("match[]", "console_helm_upgrades_total").
		Param("match[]", "console_helm_uninstalls_total").
		Param("match[]", "openshift_apps_deploymentconfigs_strategy_total").
		Param("match[]", "etcd_server_slow_apply_total").
		Param("match[]", "etcd_server_slow_read_indexes_total").
		Param("match[]", "haproxy_exporter_server_threshold").
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve most recent metrics: %v", err)
		return nil, []error{err}
	}

	records := []record.Record{
		{Name: "config/metrics", Item: marshal.RawByte(data), AlwaysStored: true},
	}

	return records, nil
}
