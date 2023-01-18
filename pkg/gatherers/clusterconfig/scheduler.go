package clusterconfig

import (
	"context"
	"fmt"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatherSchedulers Collects information about schedulers
//
// ### API Reference
// - https://docs.openshift.com/container-platform/4.9/rest_api/config_apis/scheduler-config-openshift-io-v1.html
//
// ### Sample data
// - docs/insights-archive-sample/config/schedulers/cluster.json
//
// ### Location in archive
// - `config/schedulers/cluster.json`
//
// ### Config ID
// `clusterconfig/schedulers`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherSchedulers(ctx context.Context) ([]record.Record, []error) {
	gatherConfigClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSchedulerInfo(ctx, gatherConfigClient)
}

func gatherSchedulerInfo(
	ctx context.Context, configClient configv1client.ConfigV1Interface,
) ([]record.Record, []error) {
	schedulers, err := configClient.Schedulers().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record
	for i := range schedulers.Items {
		scheduler := &schedulers.Items[i]

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/schedulers/%v", scheduler.Name),
			Item: record.ResourceMarshaller{Resource: scheduler},
		})
	}

	return records, nil
}
