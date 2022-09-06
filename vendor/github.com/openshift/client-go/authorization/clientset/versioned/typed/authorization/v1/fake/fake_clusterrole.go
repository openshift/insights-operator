// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"
	json "encoding/json"
	"fmt"

	authorizationv1 "github.com/openshift/api/authorization/v1"
	applyconfigurationsauthorizationv1 "github.com/openshift/client-go/authorization/applyconfigurations/authorization/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeClusterRoles implements ClusterRoleInterface
type FakeClusterRoles struct {
	Fake *FakeAuthorizationV1
}

var clusterrolesResource = schema.GroupVersionResource{Group: "authorization.openshift.io", Version: "v1", Resource: "clusterroles"}

var clusterrolesKind = schema.GroupVersionKind{Group: "authorization.openshift.io", Version: "v1", Kind: "ClusterRole"}

// Get takes name of the clusterRole, and returns the corresponding clusterRole object, and an error if there is any.
func (c *FakeClusterRoles) Get(ctx context.Context, name string, options v1.GetOptions) (result *authorizationv1.ClusterRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(clusterrolesResource, name), &authorizationv1.ClusterRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*authorizationv1.ClusterRole), err
}

// List takes label and field selectors, and returns the list of ClusterRoles that match those selectors.
func (c *FakeClusterRoles) List(ctx context.Context, opts v1.ListOptions) (result *authorizationv1.ClusterRoleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(clusterrolesResource, clusterrolesKind, opts), &authorizationv1.ClusterRoleList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &authorizationv1.ClusterRoleList{ListMeta: obj.(*authorizationv1.ClusterRoleList).ListMeta}
	for _, item := range obj.(*authorizationv1.ClusterRoleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested clusterRoles.
func (c *FakeClusterRoles) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(clusterrolesResource, opts))
}

// Create takes the representation of a clusterRole and creates it.  Returns the server's representation of the clusterRole, and an error, if there is any.
func (c *FakeClusterRoles) Create(ctx context.Context, clusterRole *authorizationv1.ClusterRole, opts v1.CreateOptions) (result *authorizationv1.ClusterRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(clusterrolesResource, clusterRole), &authorizationv1.ClusterRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*authorizationv1.ClusterRole), err
}

// Update takes the representation of a clusterRole and updates it. Returns the server's representation of the clusterRole, and an error, if there is any.
func (c *FakeClusterRoles) Update(ctx context.Context, clusterRole *authorizationv1.ClusterRole, opts v1.UpdateOptions) (result *authorizationv1.ClusterRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(clusterrolesResource, clusterRole), &authorizationv1.ClusterRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*authorizationv1.ClusterRole), err
}

// Delete takes name of the clusterRole and deletes it. Returns an error if one occurs.
func (c *FakeClusterRoles) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(clusterrolesResource, name, opts), &authorizationv1.ClusterRole{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeClusterRoles) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(clusterrolesResource, listOpts)

	_, err := c.Fake.Invokes(action, &authorizationv1.ClusterRoleList{})
	return err
}

// Patch applies the patch and returns the patched clusterRole.
func (c *FakeClusterRoles) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *authorizationv1.ClusterRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(clusterrolesResource, name, pt, data, subresources...), &authorizationv1.ClusterRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*authorizationv1.ClusterRole), err
}

// Apply takes the given apply declarative configuration, applies it and returns the applied clusterRole.
func (c *FakeClusterRoles) Apply(ctx context.Context, clusterRole *applyconfigurationsauthorizationv1.ClusterRoleApplyConfiguration, opts v1.ApplyOptions) (result *authorizationv1.ClusterRole, err error) {
	if clusterRole == nil {
		return nil, fmt.Errorf("clusterRole provided to Apply must not be nil")
	}
	data, err := json.Marshal(clusterRole)
	if err != nil {
		return nil, err
	}
	name := clusterRole.Name
	if name == nil {
		return nil, fmt.Errorf("clusterRole.Name must be provided to Apply")
	}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(clusterrolesResource, *name, types.ApplyPatchType, data), &authorizationv1.ClusterRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*authorizationv1.ClusterRole), err
}
