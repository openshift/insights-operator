package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/openshift/api/network/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// HostSubnetAnonymizer implements HostSubnet serialization wiht anonymization
type HostSubnetAnonymizer struct{ *networkv1.HostSubnet }

// GatherHostSubnet collects HostSubnet information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/hostsubnet.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#hostsubnet-v1-network-openshift-io
//
// Location in archive: config/hostsubnet/
func GatherHostSubnet(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		gatherNetworkClient, err := networkv1client.NewForConfig(g.gatherKubeConfig)
		if err != nil {
			return nil, []error{err}
		}
		return gatherHostSubnet(g.ctx, gatherNetworkClient)
	}
}

func gatherHostSubnet(ctx context.Context, networkClient networkv1client.NetworkV1Interface) ([]record.Record, []error) {
	hostSubnetList, err := networkClient.HostSubnets().List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	records := make([]record.Record, 0, len(hostSubnetList.Items))
	for _, h := range hostSubnetList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/hostsubnet/%s", h.Host),
			Item: HostSubnetAnonymizer{&h},
		})
	}
	return records, nil
}

// Marshal implements HostSubnet serialization
func (a HostSubnetAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(networkSerializer.LegacyCodec(networkv1.SchemeGroupVersion), a.HostSubnet)
}

// GetExtension returns extension for HostSubnet object
func (a HostSubnetAnonymizer) GetExtension() string {
	return "json"
}
