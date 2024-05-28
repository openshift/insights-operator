package clusterconfig

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"

	promcli "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (g *Gatherer) GatherAggregatedInstances(ctx context.Context) ([]record.Record, []error) {
	client, err := promcli.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return aggregatedInstances{}.gather(ctx, client)
}

// avoiding noise inside the clusterconfig package with more 'private' functions
type aggregatedInstances struct {
	Prometheuses  []string `json:"prometheuses"`
	Alertmanagers []string `json:"alertmanagers"`
}

func (ai aggregatedInstances) gather(ctx context.Context, client promcli.Interface) ([]record.Record, []error) {
	const Filename = "config/aggregated/custom_prometheuses_alertmanagers"
	const SystemNamespace = "openshift-monitoring"

	errs := []error{}

	prometheusList, err := client.MonitoringV1().Prometheuses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		errs = append(errs, err)
	}

	alertManagers, err := client.MonitoringV1().Alertmanagers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errs
	}

	for _, prom := range prometheusList.Items {
		if prom.GetNamespace() != SystemNamespace {
			ai.Prometheuses = append(ai.Prometheuses, prom.GetName())
		}
	}

	for _, am := range alertManagers.Items {
		if am.GetNamespace() != SystemNamespace {
			ai.Alertmanagers = append(ai.Alertmanagers, am.GetName())
		}
	}

	records := []record.Record{{Name: Filename, Item: record.JSONMarshaller{Object: ai}}}
	// for _, prom := range prometheusList.Items {
	// 	if prom.GetNamespace() != SystemNamespace {
	// 		records = append(records, record.Record{Name: Filename, Item: record.JSONMarshaller{Object: ai}})
	// 	}
	// }

	return records, nil
}
