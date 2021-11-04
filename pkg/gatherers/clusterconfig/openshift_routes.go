package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// GatherOpenshiftRoutes collects `route.openshift.io/v1` resources.
//
// API Reference:
//   https://github.com/openshift/api/blob/master/route/v1/types.go
//
// * Location in archive: config/routes/<namespace>/<name>.json
// * Since versions:
//   * 4.9+
func (g *Gatherer) GatherOpenshiftRoutes(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenshiftRoutes(ctx, gatherDynamicClient)
}

func gatherOpenshiftRoutes(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	routeResources, err := dynamicClient.Resource(openshiftRouteResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", openshiftRouteResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range routeResources.Items {
		item := routeResources.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/routes/%s/%s", item.GetNamespace(), item.GetName()),
			Item: record.ResourceMarshaller{Resource: &item},
		})
	}
	return records, nil
}
