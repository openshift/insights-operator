package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/client-go/dynamic"
)

// GatherTempoStackCR Collects maximum of 5 `tempostacks.tempo.grafana.com` custom resources installed in the cluster.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/tempo.grafana.com/openshift-operators/simpletest.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.15.0 | config/tempo.grafana.com/{namespace}/{name}.json 					    |
//
// ### Config ID
// `clusterconfig/tempo_stack`
//
// ### Released version
// - 4.15.0
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherTempoStackCR(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherTempoStackCR(ctx, gatherDynamicClient)
}

func gatherTempoStackCR(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	tempoList, err := dynamicClient.Resource(tempoStackResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var errs []error
	// Limit the number of gathered tempostacks.tempo.grafana.com resources
	var limit = 5
	records := make([]record.Record, 0, limit)
	for i := range tempoList.Items {
		t := tempoList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/%s/%s/%s", tempoStackResource.Group, t.GetNamespace(), t.GetName()),
			Item: record.ResourceMarshaller{Resource: &t},
		})
		// limit the gathered records
		if len(records) == limit {
			err := fmt.Errorf("limit %d for number of gathered %s resources exceeded", limit, tempoStackResource.GroupResource())
			errs = append(errs, err)
			break
		}
	}

	return records, errs
}
