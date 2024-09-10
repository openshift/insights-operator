// nolint: dupl
package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// GatherNodeNetworkConfigurationPolicy Collects cluster scope "nodenetworkconfigurationpolicy.nmstate.io/v1"
// resources
//
// ### API Reference
// - https://github.com/nmstate/kubernetes-nmstate/blob/main/api/v1/nodenetworkconfigurationpolicy_types.go
//
// ### Sample data
// - docs/insights-archive-sample/cluster-scoped-resources/nmstate.io/nodenetworkconfigurationpolicies/etcd-quorum-guard.json
//
// ### Location in archive
// - `cluster-scoped-resources/nmstate.io/nodenetworkconfigurationpolicies/{name}.json`
//
// ### Config ID
// `clusterconfig/nodenetworkconfigurationpolicies`
//
// ### Released version
// - 4.18.0
//
// ### Backported versions
//
// ### Changes
func (g *Gatherer) GatherNodeNetworkConfigurationPolicy(ctx context.Context) ([]record.Record, []error) {
	dynCli, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherNodeNetworkConfigurationPolicy(ctx, dynCli)
}

func gatherNodeNetworkConfigurationPolicy(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	nodeNetConfPoliciesList, err := dynamicClient.Resource(nodeNetConfPoliciesV1GVR).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	records := []record.Record{}
	for i := range nodeNetConfPoliciesList.Items {
		nodeNetworkConfigurationPolicy := nodeNetConfPoliciesList.Items[i]
		records = append(records, record.Record{
			Name: fmt.Sprintf("cluster-scoped-resources/nmstate.io/nodenetworkconfigurationpolicies/%s", nodeNetworkConfigurationPolicy.GetName()),
			Item: record.ResourceMarshaller{Resource: &nodeNetworkConfigurationPolicy},
		})
	}
	return records, nil
}
