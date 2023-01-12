package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherCostManagementMetricsConfigs Collects CostManagementMetricsConfigs definitions.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/cost_management_metrics_configs
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.10.0 | config/cost_management_metrics_configs/{name}.json 		|
//
// ### Config ID
// `clusterconfig/cost_management_metrics_configs`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// - 4.8.27+
// - 4.9.13+
//
// ### Changes
// None
func (g *Gatherer) GatherCostManagementMetricsConfigs(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherCostManagementMetricsConfigs(ctx, gatherDynamicClient)
}

func gatherCostManagementMetricsConfigs(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	mcList, err := dynamicClient.Resource(costManagementMetricsConfigResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	records := []record.Record{}
	var errs []error
	for i := range mcList.Items {
		mc := mcList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/cost_management_metrics_configs/%s", mc.GetName()),
			Item: record.ResourceMarshaller{Resource: &mc},
		})
	}
	if len(errs) > 0 {
		return records, errs
	}
	return records, nil
}
