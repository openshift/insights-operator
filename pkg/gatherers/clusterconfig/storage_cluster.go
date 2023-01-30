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

// GatherStorageCluster collects `storageclusters.ocs.openshift.io` resources
//
// The Kubernetes API https://github.com/red-hat-storage/ocs-operator/blob/main/api/v1/storagecluster_types.go
//
// * Location of serviceaccounts in archive: config/storage/{namespace}/storageclusters/{name}.json
// * See: docs/insights-archive-sample/config/storage/openshift-storage/storageclusters/ocs-storagecluster.json
// * Id in config: clusterconfig/storage_cluster
// * Since versions:
//   - 4.11.0
func (g *Gatherer) GatherStorageCluster(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherStorageCluster(ctx, gatherDynamicClient)
}

func gatherStorageCluster(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	storageResourceList, err := dynamicClient.Resource(storageClusterResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", storageClusterResource, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range storageResourceList.Items {
		item := storageResourceList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/storage/%s/%s/%s", item.GetNamespace(), storageClusterResource.Resource, item.GetName()),
			Item: record.ResourceMarshaller{Resource: &item},
		})
	}
	return records, nil
}
