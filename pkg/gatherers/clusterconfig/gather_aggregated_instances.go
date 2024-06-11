package clusterconfig

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"

	promcli "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatherAggregatedMonitoringCRNames Collects instances outside of the `openshift-monitoring` of the following custom resources:
// - Kind: `Prometheus` Group: `monitoring.coreos.com`
// - Kind: `AlertManager` Group: `monitoring.coreos.com`
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
// `clusterconfig/aggregated_monitoring_cr_names`
//
// ### Released version
// - 4.16
//
// ### Backported versions
// TBD
//
// ### Changes
// None
func (g *Gatherer) GatherAggregatedMonitoringCRNames(ctx context.Context) ([]record.Record, []error) {
	client, err := promcli.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return monitoringCRNames{}.gather(ctx, client)
}

type monitoringCRNames struct {
	Prometheuses  []string `json:"prometheuses"`
	Alertmanagers []string `json:"alertmanagers"`
}

// gather returns records for all Prometheus and Alertmanager instances that exist outside the openshift-monitoring namespace.
// It could instead return a collection of errors found when trying to get those instances.
func (mn monitoringCRNames) gather(ctx context.Context, client promcli.Interface) ([]record.Record, []error) {
	const Filename = "aggregated/custom_prometheuses_alertmanagers"

	errs := []error{}
	prometheusList, err := mn.getOutcastedPrometheuses(ctx, client)
	if err != nil {
		errs = append(errs, err)
	}

	alertManagersList, err := mn.getOutcastedAlertManagers(ctx, client)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errs
	}

	// De not return an empty file if no Custom Resources were found
	if len(prometheusList) == 0 && len(alertManagersList) == 0 {
		return []record.Record{}, nil
	}

	mn.Prometheuses = prometheusList
	mn.Alertmanagers = alertManagersList

	return []record.Record{{Name: Filename, Item: record.JSONMarshaller{Object: mn}}}, nil
}

// getOutcastedAlertManagers returns a collection of AlertManagers names, if any, from other than the openshift-monitoring namespace
// or an error if it couldn't retrieve them
func (mn monitoringCRNames) getOutcastedAlertManagers(ctx context.Context, client promcli.Interface) ([]string, error) {
	alertManagersList, err := client.MonitoringV1().Alertmanagers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	amNames := []string{}
	for i := range alertManagersList.Items {
		alertMgr := alertManagersList.Items[i]
		if alertMgr.GetNamespace() != monitoringNamespace {
			amNames = append(amNames, alertMgr.GetName())
		}
	}

	return amNames, nil
}

// getOutcastedPrometheuses returns a collection of Prometheus names, if any, from other than the openshift-monitoring namespace
// or an error if it couldn't retrieve them
func (mn monitoringCRNames) getOutcastedPrometheuses(ctx context.Context, client promcli.Interface) ([]string, error) {
	prometheusList, err := client.MonitoringV1().Prometheuses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	promNames := []string{}
	for i := range prometheusList.Items {
		prom := prometheusList.Items[i]
		if prom.GetNamespace() != monitoringNamespace {
			promNames = append(promNames, prom.GetName())
		}
	}

	return promNames, nil
}
