package clusterconfig

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherHostSubnet collects HostSubnet information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/hostsubnet.go
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#hostsubnet-v1-network-openshift-io
//
// Location in archive: config/hostsubnet/
// Id in config: host_subnets
func GatherHostSubnet(g *Gatherer, c chan<- gatherResult) {
	defer close(c)
	gatherNetworkClient, err := networkv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		c <- gatherResult{nil, []error{err}}
		return
	}
	records, errors := gatherHostSubnet(g.ctx, gatherNetworkClient)
	c <- gatherResult{records, errors}
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
			Item: record.JSONMarshaller{Object: h},
		})
	}
	return records, nil
}
