package clusterconfig

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"

	promcli "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatherAggregatedInstances Collects instances of `Prometheus` and `AlertManager` deployments
// that are outside of the `openshift-monitoring` namespace
//
// ### API Reference
// - https://docs.openshift.com/container-platform/4.13/rest_api/monitoring_apis/alertmanager-monitoring-coreos-com-v1.html
// - https://docs.openshift.com/container-platform/4.13/rest_api/monitoring_apis/prometheus-monitoring-coreos-com-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/aggregated/custom_prometheuses_alertmanagers.json
//
// ### Location in archive
// - `aggregated/custom_prometheuses_alertmanagers.json`
//
// ### Config ID
// `clusterconfig/aggregated_instances`
//
// ### Released version
// - 4.16
//
// ### Backported versions
// TBD
//
// ### Changes
// None
func (g *Gatherer) GatherAggregatedInstances(ctx context.Context) ([]record.Record, []error) {
	client, err := promcli.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return aggregatedInstances{}.gather(ctx, client)
}

type aggregatedInstances struct {
	Prometheuses  []string `json:"prometheuses"`
	Alertmanagers []string `json:"alertmanagers"`
}

// gather returns records for all Prometheus and Alertmanager instances that exist outside the openshift-monitoring namespace.
// It could instead return a collection of errors found when trying to get those instances.
func (ai aggregatedInstances) gather(ctx context.Context, client promcli.Interface) ([]record.Record, []error) {
	const Filename = "aggregated/custom_prometheuses_alertmanagers"

	errs := []error{}
	prometheusList, err := ai.getOutcastedPrometheuses(ctx, client)
	if err != nil {
		errs = append(errs, err)
	}

	alertManagersList, err := ai.getOutcastedAlertManagers(ctx, client)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errs
	}

	ai.Prometheuses = prometheusList
	ai.Alertmanagers = alertManagersList

	return []record.Record{{Name: Filename, Item: record.JSONMarshaller{Object: ai}}}, nil
}

// getOutcastedAlertManagers returns a collection of AlertManagers names, if any, from other than the openshift-monitoring namespace
// or an error if it couldn't retrieve them
func (ai aggregatedInstances) getOutcastedAlertManagers(ctx context.Context, client promcli.Interface) ([]string, error) {
	alertManagersList, err := client.MonitoringV1().Alertmanagers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	amNames := []string{}
	for i := range alertManagersList.Items {
		alertMgr := alertManagersList.Items[i]
		if alertMgr.GetNamespace() != MonitoringNamespace {
			amNames = append(amNames, alertMgr.GetName())
		}
	}

	return amNames, nil
}

// getOutcastedPrometheuses returns a collection of Prometheus names, if any, from other than the openshift-monitoring namespace
// or an error if it couldn't retrieve them
func (ai aggregatedInstances) getOutcastedPrometheuses(ctx context.Context, client promcli.Interface) ([]string, error) {
	prometheusList, err := client.MonitoringV1().Prometheuses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	promNames := []string{}
	for i := range prometheusList.Items {
		prom := prometheusList.Items[i]
		if prom.GetNamespace() != MonitoringNamespace {
			promNames = append(promNames, prom.GetName())
		}
	}

	return promNames, nil
}
