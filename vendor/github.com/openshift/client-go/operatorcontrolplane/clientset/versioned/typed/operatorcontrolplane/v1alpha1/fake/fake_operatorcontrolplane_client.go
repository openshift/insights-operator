// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/openshift/client-go/operatorcontrolplane/clientset/versioned/typed/operatorcontrolplane/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeControlplaneV1alpha1 struct {
	*testing.Fake
}

func (c *FakeControlplaneV1alpha1) PodNetworkConnectivityChecks(namespace string) v1alpha1.PodNetworkConnectivityCheckInterface {
	return newFakePodNetworkConnectivityChecks(c, namespace)
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeControlplaneV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
