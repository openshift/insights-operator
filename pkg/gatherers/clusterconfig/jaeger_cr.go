package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// GatherJaegerCR Collects maximum of 5 `jaegers.jaegertracing.io` custom resources installed in the cluster.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/jaegertracing.io/jaeger1.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.10.0 | config/.json 					                        	|
//
// ### Config ID
// `clusterconfig/jaegers`
//
// ### Released version
// - 4.10.0
//
// ### Backported versions
// None
//
// ### Notes
// None
func (g *Gatherer) GatherJaegerCR(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherJaegerCR(ctx, gatherDynamicClient)
}

func gatherJaegerCR(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	jaegersList, err := dynamicClient.Resource(jaegerResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var errs []error
	// Limit the number of gathered jaegers.jaegertracing.io resources
	var limit = 5
	records := make([]record.Record, 0, limit)
	for i := range jaegersList.Items {
		j := jaegersList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/%s/%s", jaegerResource.Group, j.GetName()),
			Item: record.ResourceMarshaller{Resource: &j},
		})
		// limit the gathered records
		if len(records) == limit {
			err := fmt.Errorf("limit %d for number of gathered %s resources exceeded", limit, jaegerResource.GroupResource())
			errs = append(errs, err)
			break
		}
	}

	return records, errs
}
