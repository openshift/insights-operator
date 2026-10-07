package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherClusterServingRuntimes Collects cluster-scoped ClusterServingRuntime resources
// from the KServe operator. Only selected non-sensitive fields are collected: supported
// model formats, container name/image/args, multi-model flag, protocol versions and
// worker parallelism.
//
// The containers env field is never collected as it commonly holds tokens and API keys.
// Values of container args are anonymized when the flag name looks sensitive or the
// value looks like a URI, since args may carry tokens or object storage paths.
//
// ### API Reference
// - https://kserve.github.io/website/docs/reference/crd-api#clusterservingruntime
//
// ### Sample data
// - docs/insights-archive-sample/config/serving.kserve.io/clusterservingruntimes.json
//
// ### Location in archive
// - `config/serving.kserve.io/clusterservingruntimes.json`
//
// ### Config ID
// `clusterconfig/cluster_serving_runtimes`
//
// ### Released version
// - 5.1
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherClusterServingRuntimes(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherClusterServingRuntimes(ctx, dynamicClient)
}

func gatherClusterServingRuntimes(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	list, err := dynamicClient.Resource(clusterServingRuntimeGVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(list.Items) == 0 {
		return nil, nil
	}

	runtimes, errs := extractServingRuntimes(list.Items, clusterServingRuntimeGVR)

	return []record.Record{{
		Name: "config/serving.kserve.io/clusterservingruntimes",
		Item: record.JSONMarshaller{Object: runtimes},
	}}, errs
}
