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

// GatherOpenshiftLogging collects `logging.openshift.io` resources
// from Openshift Logging Stack.
//
// * Location in archive: config/logging/<namespace>/<name>.json
// * Since versions:
//   * 4.9+
func (g *Gatherer) GatherOpenshiftLogging(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenshiftLogging(ctx, gatherDynamicClient)
}

func gatherOpenshiftLogging(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	elasticsearchList, err := dynamicClient.Resource(openshiftLoggingResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", openshiftLoggingResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	for _, i := range elasticsearchList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/logging/%s/%s", i.GetNamespace(), i.GetName()),
			Item: record.JSONMarshaller{Object: i.Object},
		})
	}
	return records, nil
}
