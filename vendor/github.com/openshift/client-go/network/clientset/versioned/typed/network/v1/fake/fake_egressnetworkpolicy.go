// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"
	json "encoding/json"
	"fmt"

	networkv1 "github.com/openshift/api/network/v1"
	applyconfigurationsnetworkv1 "github.com/openshift/client-go/network/applyconfigurations/network/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeEgressNetworkPolicies implements EgressNetworkPolicyInterface
type FakeEgressNetworkPolicies struct {
	Fake *FakeNetworkV1
	ns   string
}

var egressnetworkpoliciesResource = schema.GroupVersionResource{Group: "network.openshift.io", Version: "v1", Resource: "egressnetworkpolicies"}

var egressnetworkpoliciesKind = schema.GroupVersionKind{Group: "network.openshift.io", Version: "v1", Kind: "EgressNetworkPolicy"}

// Get takes name of the egressNetworkPolicy, and returns the corresponding egressNetworkPolicy object, and an error if there is any.
func (c *FakeEgressNetworkPolicies) Get(ctx context.Context, name string, options v1.GetOptions) (result *networkv1.EgressNetworkPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(egressnetworkpoliciesResource, c.ns, name), &networkv1.EgressNetworkPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*networkv1.EgressNetworkPolicy), err
}

// List takes label and field selectors, and returns the list of EgressNetworkPolicies that match those selectors.
func (c *FakeEgressNetworkPolicies) List(ctx context.Context, opts v1.ListOptions) (result *networkv1.EgressNetworkPolicyList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(egressnetworkpoliciesResource, egressnetworkpoliciesKind, c.ns, opts), &networkv1.EgressNetworkPolicyList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &networkv1.EgressNetworkPolicyList{ListMeta: obj.(*networkv1.EgressNetworkPolicyList).ListMeta}
	for _, item := range obj.(*networkv1.EgressNetworkPolicyList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested egressNetworkPolicies.
func (c *FakeEgressNetworkPolicies) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(egressnetworkpoliciesResource, c.ns, opts))

}

// Create takes the representation of a egressNetworkPolicy and creates it.  Returns the server's representation of the egressNetworkPolicy, and an error, if there is any.
func (c *FakeEgressNetworkPolicies) Create(ctx context.Context, egressNetworkPolicy *networkv1.EgressNetworkPolicy, opts v1.CreateOptions) (result *networkv1.EgressNetworkPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(egressnetworkpoliciesResource, c.ns, egressNetworkPolicy), &networkv1.EgressNetworkPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*networkv1.EgressNetworkPolicy), err
}

// Update takes the representation of a egressNetworkPolicy and updates it. Returns the server's representation of the egressNetworkPolicy, and an error, if there is any.
func (c *FakeEgressNetworkPolicies) Update(ctx context.Context, egressNetworkPolicy *networkv1.EgressNetworkPolicy, opts v1.UpdateOptions) (result *networkv1.EgressNetworkPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(egressnetworkpoliciesResource, c.ns, egressNetworkPolicy), &networkv1.EgressNetworkPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*networkv1.EgressNetworkPolicy), err
}

// Delete takes name of the egressNetworkPolicy and deletes it. Returns an error if one occurs.
func (c *FakeEgressNetworkPolicies) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(egressnetworkpoliciesResource, c.ns, name, opts), &networkv1.EgressNetworkPolicy{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeEgressNetworkPolicies) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(egressnetworkpoliciesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &networkv1.EgressNetworkPolicyList{})
	return err
}

// Patch applies the patch and returns the patched egressNetworkPolicy.
func (c *FakeEgressNetworkPolicies) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *networkv1.EgressNetworkPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(egressnetworkpoliciesResource, c.ns, name, pt, data, subresources...), &networkv1.EgressNetworkPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*networkv1.EgressNetworkPolicy), err
}

// Apply takes the given apply declarative configuration, applies it and returns the applied egressNetworkPolicy.
func (c *FakeEgressNetworkPolicies) Apply(ctx context.Context, egressNetworkPolicy *applyconfigurationsnetworkv1.EgressNetworkPolicyApplyConfiguration, opts v1.ApplyOptions) (result *networkv1.EgressNetworkPolicy, err error) {
	if egressNetworkPolicy == nil {
		return nil, fmt.Errorf("egressNetworkPolicy provided to Apply must not be nil")
	}
	data, err := json.Marshal(egressNetworkPolicy)
	if err != nil {
		return nil, err
	}
	name := egressNetworkPolicy.Name
	if name == nil {
		return nil, fmt.Errorf("egressNetworkPolicy.Name must be provided to Apply")
	}
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(egressnetworkpoliciesResource, c.ns, *name, types.ApplyPatchType, data), &networkv1.EgressNetworkPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*networkv1.EgressNetworkPolicy), err
}
