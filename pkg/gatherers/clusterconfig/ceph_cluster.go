// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherCephCluster collects statuses of the`cephclusters.ceph.rook.io` resources
// from Openshift Data Foundation Stack.
//
// The Kubernetes API https://github.com/rook/rook/blob/master/pkg/apis/ceph.rook.io/v1/types.go
//
// * Location of serviceaccounts in archive: config/storage/{namespace}/cephclusters/{name}.json
// * See: docs/insights-archive-sample/config/storage/openshift-storage/cephclusters/ocs-storagecluster-cephcluster.json
// * Id in config: clusterconfig/ceph_cluster
// * Since versions:
//   - 4.8.49
//   - 4.9.48
//   - 4.10.31
//   - 4.11.2
//   - 4.12
func (g *Gatherer) GatherCephCluster(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherCephCluster(ctx, gatherDynamicClient)
}

func gatherCephCluster(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	cephClusterList, err := dynamicClient.Resource(cephClustereResource).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", gatherCephCluster, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range cephClusterList.Items {
		item := &cephClusterList.Items[i]
		status := item.Object["status"]
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/storage/%s/%s/%s", item.GetNamespace(), cephClustereResource.Resource, item.GetName()),
			Item: record.JSONMarshaller{Object: status},
		})
	}
	return records, nil
}
