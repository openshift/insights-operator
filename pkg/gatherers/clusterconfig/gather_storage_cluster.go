package clusterconfig

// nolint: dupl

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherStorageCluster Collects `storageclusters.ocs.openshift.io` resources
//
// ### API Reference
// - https://github.com/red-hat-storage/ocs-operator/blob/main/api/v1/storagecluster_types.go
//
// ### Sample data
// - docs/insights-archive-sample/config/storage/openshift-storage/storageclusters/ocs-storagecluster.json
//
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | < 4.12.0  | config/storage/{namespace}/{name}.json 					|
// | >= 4.12.0 | config/storage/{namespace}/storageclusters/{name}.json 	|
//
// ### Config ID
// `clusterconfig/storage_cluster`
//
// ### Released version
// - 4.11.0
//
// ### Backported versions
// None
//
// ### Changes
// - Renamed from `OpenshiftStorage` to `StorageCluster` in version `4.12.0+`
// - Config ID changed from `clusterconfig/openshift_storage` to `clusterconfig/storage_cluster` in version `4.12.0+`
// - In OCP 4.11 and OCP 4.12, the location of gathered data collides with data gathered by the
// CephCluster](#CephCluster) gatherer. It is practically impossible to tell the two resources apart. Use with caution.
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
