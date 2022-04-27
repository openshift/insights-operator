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

// GatherOpenshiftStorage collects `storageclusters.ocs.openshift.io` resources
// from Openshift Data Foundation Stack.
//
// API Reference:
//   https://github.com/red-hat-storage/ocs-operator/blob/main/api/v1/storagecluster_types.go
//
// * Location in archive: config/storage/<namespace>/<name>.json
// * Id in config: clusterconfig/openshift_storage
// * Since versions:
//   * 4.11+
func (g *Gatherer) GatherOpenshiftStorage(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherOpenshiftStorage(ctx, gatherDynamicClient)
}

func gatherOpenshiftStorage(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	storageResourceList, err := dynamicClient.Resource(openshiftStorageResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", openshiftStorageResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range storageResourceList.Items {
		item := storageResourceList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/storage/%s/%s", item.GetNamespace(), item.GetName()),
			Item: record.ResourceMarshaller{Resource: &item},
		})
	}
	return records, nil
}
