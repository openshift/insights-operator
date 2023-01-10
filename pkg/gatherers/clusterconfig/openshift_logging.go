// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherOpenshiftLogging Collects `logging.openshift.io` resources
// from Openshift Logging Stack.
//
// ### API Reference
// - https://github.com/openshift/cluster-logging-operator/blob/master/pkg/apis/logging/v1/clusterlogging_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/logging/openshift-logging/instance.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.9.0  | config/logging/{namespace}/{name}.json 		            |
//
// ### Config ID
// `clusterconfig/openshift_logging`
//
// ### Released version
// - 4.9.0
//
// ### Backported versions
// None
//
// ### Notes
// None
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
