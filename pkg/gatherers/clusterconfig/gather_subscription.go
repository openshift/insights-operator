package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherSubscription Collects `Subscription` from all namespaces.
//
// ### API Reference
// - https://github.com/operator-framework/api/blob/master/crds/operators.coreos.com_subscriptions.yaml
//
// ### Sample data
// - docs/insights-archive-sample/config/subscriptions/community-kubevirt-hyperconverged.json
//
// ### Location in archive
// - `config/subscriptions/{name}.json`
//
// ### Config ID
// `clusterconfig/subscriptions`
//
// ### Released version
// - 4.22
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherSubscription(ctx context.Context) ([]record.Record, []error) {
	dynamicClient, err := dynamic.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherSubscriptions(ctx, dynamicClient)
}

func gatherSubscriptions(ctx context.Context, dynamicClient dynamic.Interface) ([]record.Record, []error) {
	gvr := schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}

	// List all resources
	subscriptionList, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var records []record.Record

	for i, rc := range subscriptionList.Items {
		// Drop status field that is not needed
		delete(subscriptionList.Items[i].Object, "status")

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/subscriptions/%s", rc.GetName()),
			Item: record.ResourceMarshaller{Resource: &subscriptionList.Items[i]},
		})
	}

	return records, nil
}
