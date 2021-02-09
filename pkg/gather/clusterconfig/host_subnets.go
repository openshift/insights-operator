package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/openshift/api/network/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherHostSubnet collects HostSubnet information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/hostsubnet.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#hostsubnet-v1-network-openshift-io
//
// Location in archive: config/hostsubnet/
func GatherHostSubnet(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {

		hsList, err := i.networkClient.HostSubnets().List(i.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		records := make([]record.Record, 0, len(hsList.Items))
		for i, h := range hsList.Items {
			records = append(records, record.Record{Name: fmt.Sprintf("config/hostsubnet/%s", h.Host), Item: HostSubnetAnonymizer{&hsList.Items[i]}})
		}
		return records, nil
	}
}


// HostSubnetAnonymizer implements HostSubnet serialization wiht anonymization
type HostSubnetAnonymizer struct{ *networkv1.HostSubnet }

// Marshal implements HostSubnet serialization
func (a HostSubnetAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(networkSerializer.LegacyCodec(networkv1.SchemeGroupVersion), a.HostSubnet)
}

// GetExtension returns extension for HostSubnet object
func (a HostSubnetAnonymizer) GetExtension() string {
	return "json"
}
