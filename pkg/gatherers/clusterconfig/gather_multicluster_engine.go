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

// GatherMultiClusterEngine Collects `multiclusterengines.multicluster.openshift.io` resources.
// These resources are created when the Multicluster Engine for Kubernetes operator is installed.
// Usually only one MultiClusterEngine object exists in a cluster.
//
// ### API Reference
// - https://github.com/stolostron/backplane-operator/blob/main/api/v1/multiclusterengine_types.go
//
// ### Sample data
// - docs/insights-archive-sample/cluster-scoped-resources/multicluster.openshift.io/multiclusterengines/engine.json
//
// ### Location in archive
// - `cluster-scoped-resources/multicluster.openshift.io/multiclusterengines/{name}.json`
//
// ### Config ID
// `clusterconfig/multicluster_engine`
//
// ### Released version
// - 5.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherMultiClusterEngine(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherMultiClusterEngine(ctx, dynamicClient)
}

func gatherMultiClusterEngine(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	mceList, err := dynamicClient.Resource(multiClusterEngineGVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		klog.V(2).Infof("Unable to list %s resource due to: %s", multiClusterEngineGVR, err)
		return nil, []error{err}
	}

	var records []record.Record
	for i := range mceList.Items {
		item := &mceList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("cluster-scoped-resources/%s/%s/%s",
				multiClusterEngineGVR.Group,
				multiClusterEngineGVR.Resource,
				item.GetName()),
			Item: record.ResourceMarshaller{Resource: item},
		})
	}
	return records, nil
}
