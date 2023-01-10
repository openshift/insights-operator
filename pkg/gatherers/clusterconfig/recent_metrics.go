package clusterconfig

import (
	"context"
	"fmt"
	"io"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
)

const (
	// metricsAlertsLinesLimit is the maximal number of lines read from monitoring Prometheus
	// 500 KiB of alerts is limit, one alert line has typically 450 bytes => 1137 lines.
	// This number has been rounded to 1000 for simplicity.
	// Formerly, the `500 * 1024 / 450` expression was used instead.
	metricsAlertsLinesLimit = 1000
)

// GatherMostRecentMetrics Collects cluster Federated Monitoring metrics.
//
// The GET REST query to URL /federate
// Gathered metrics:
//   - virt_platform
//   - etcd_object_counts
//   - cluster_installer
//   - vsphere_node_hw_version_total
//   - namespace CPU and memory usage
//   - console_helm_installs_total
//   - console_helm_upgrades_total
//   - console_helm_uninstalls_total
//   - followed by at most 1000 lines of ALERTS metric
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/metrics
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.3.0  | config/metrics 					                        |
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
// ### Notes
// **Additional data**:
// - "etcd_object_counts": 4.3+
// - "cluster_installer": 4.3+
// - "ALERTS": 4.3+
// - "namespace:container_cpu_usage_seconds_total:sum_rate": 4.5+
// - "namespace:container_memory_usage_bytes:sum": 4.5+
// - "virt_platform metric": 4.6.34+, 4.7.16+, 4.8+
// - "vsphere_node_hw_version_total": 4.7.11+, 4.8+
// - "console_helm_installs_total": 4.11+
// - "console_helm_upgrades_total": 4.12+
// - "console_helm_uninstalls_total": 4.12+
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
		Param("match[]", "console_helm_installs_total").
		Param("match[]", "console_helm_upgrades_total").
		Param("match[]", "console_helm_uninstalls_total").
		DoRaw(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve most recent metrics: %v", err)
		return nil, []error{err}
	}

	rsp, err := metricsClient.Get().AbsPath("federate").
		Param("match[]", "ALERTS").
		Stream(ctx)
	if err != nil {
		klog.Errorf("Unable to retrieve most recent alerts from metrics: %v", err)
		return nil, []error{err}
	}
	r := utils.NewLineLimitReader(rsp, metricsAlertsLinesLimit)
	alerts, err := io.ReadAll(r)
	if err != nil && err != io.EOF {
		klog.Errorf("Unable to read most recent alerts from metrics: %v", err)
		return nil, []error{err}
	}

	remainingAlertLines, err := utils.CountLines(rsp)
	if err != nil && err != io.EOF {
		klog.Errorf("Unable to count truncated lines of alerts metric: %v", err)
		return nil, []error{err}
	}
	totalAlertCount := r.GetTotalLinesRead() + remainingAlertLines

	// # ALERTS <Total Alerts Lines>/<Alerts Line Limit>
	// The total number of alerts will typically be greater than the true number of alerts by 2
	// because the `# TYPE ALERTS untyped` header and the final empty line are counter in.
	data = append(data, []byte(fmt.Sprintf("# ALERTS %d/%d\n", totalAlertCount, metricsAlertsLinesLimit))...)
	data = append(data, alerts...)
	records := []record.Record{
		{Name: "config/metrics", Item: marshal.RawByte(data)},
	}

	return records, nil
}
