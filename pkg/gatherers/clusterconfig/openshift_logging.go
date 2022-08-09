// nolint: dupl
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
// API Reference:
//
//	https://github.com/openshift/cluster-logging-operator/blob/master/pkg/apis/logging/v1/clusterlogging_types.go
//
// * Location in archive: config/logging/<namespace>/<name>.json
// * Id in config: clusterconfig/openshift_logging
// * Since versions:
//   - 4.9+
func (g *Gatherer) GatherOpenshiftLogging(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenshiftLogging(ctx, gatherDynamicClient)
}

func gatherOpenshiftLogging(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	loggingResourceList, err := dynamicClient.Resource(openshiftLoggingResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", openshiftLoggingResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range loggingResourceList.Items {
		item := loggingResourceList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/logging/%s/%s", item.GetNamespace(), item.GetName()),
			Item: record.ResourceMarshaller{Resource: &item},
		})
	}
	return records, nil
}
