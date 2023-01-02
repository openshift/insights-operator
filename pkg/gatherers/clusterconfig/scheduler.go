package clusterconfig

import (
	"context"
	"fmt"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatherSchedulers collects information about schedulers
//
// The API:
//
//	https://docs.openshift.com/container-platform/4.9/rest_api/config_apis/scheduler-config-openshift-io-v1.html
//
// * Location in archive: config/schedulers/cluster.json
// * Since versions:
//   - 4.10+
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
