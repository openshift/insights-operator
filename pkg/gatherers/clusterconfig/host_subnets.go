package clusterconfig

import (
	"context"
	"fmt"

	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherHostSubnet collects HostSubnet information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/hostsubnet.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#hostsubnet-v1-network-openshift-io
//
// * Location in archive: config/hostsubnet/
// * Id in config: host_subnets
// * Since versions:
//   * 4.4.29+
//   * 4.5.15+
//   * 4.6+
func (g *Gatherer) GatherHostSubnet(ctx context.Context) ([]record.Record, []error) {
	gatherNetworkClient, err := networkv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherHostSubnet(ctx, gatherNetworkClient)
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

	for i := range hostSubnetList.Items {
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/hostsubnet/%s", hostSubnetList.Items[i].Host),
			Item: record.ResourceMarshaller{Resource: &hostSubnetList.Items[i]},
		})
	}
	return records, nil
}
