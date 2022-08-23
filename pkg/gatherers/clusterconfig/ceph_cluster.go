//nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// GatherCephCluster collects statuses of the`cephclusters.ceph.rook.io` resources
// from Openshift Data Foundation Stack.
//
// API Reference:
//   https://github.com/rook/rook/blob/master/pkg/apis/ceph.rook.io/v1/types.go
//
// * Location in archive: config/storage/<namespace>/<name>.json
// * Id in config: clusterconfig/ceph_cluster
// * Since versions:
//   * 4.12+
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
			Name: fmt.Sprintf("config/storage/%s/%s", item.GetNamespace(), item.GetName()),
			Item: record.JSONMarshaller{Object: status},
		})
	}
	return records, nil
}
