package clusterconfig //nolint:dupl

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherMultiClusterHub Collects MultiClusterHub resources from the
// operator.open-cluster-management.io/v1 API group, which are created after the
// Red Hat Advanced Cluster Management for Kubernetes operator is installed.
//
// ### API Reference
// - https://github.com/stolostron/multiclusterhub-operator/blob/main/api/v1/multiclusterhub_types.go
//
// ### Sample data
// - docs/insights-archive-sample/cluster-scoped-resources/operator.open-cluster-management.io/multiclusterhubs/multiclusterhub.json
//
// ### Location in archive
// - `cluster-scoped-resources/operator.open-cluster-management.io/multiclusterhubs/{name}.json`
//
// ### Config ID
// `clusterconfig/multiclusterhubs`
//
// ### Released version
// - 5.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherMultiClusterHub(ctx context.Context) ([]record.Record, []error) {
	gatherDynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMultiClusterHub(ctx, gatherDynamicClient)
}

func gatherMultiClusterHub(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	multiClusterHubList, err := dynamicClient.Resource(multiClusterHubGVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", multiClusterHubGVR, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range multiClusterHubList.Items {
		item := &multiClusterHubList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("cluster-scoped-resources/%s/%s/%s",
				multiClusterHubGVR.Group,
				multiClusterHubGVR.Resource,
				item.GetName(),
			),
			Item: record.ResourceMarshaller{Resource: item},
		})
	}
	return records, nil
}
