package clusterconfig

import (
	"context"

	networkv1 "github.com/openshift/api/network/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

type netNamespace struct {
	Name      string                           `json:"name"`
	EgressIPs []networkv1.NetNamespaceEgressIP `json:"egressIPs"`
	NetID     uint32                           `json:"netID"`
}

// GatherNetNamespace Collects NetNamespaces networking information.
//
// ### API Reference
// - https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/netnamespace.go
//
// ### Sample data
// - docs/insights-archive-sample/config/netnamespaces.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.6.20 | config/netnamespaces.json				                    |
//
// ### Config ID
// `clusterconfig/netnamespaces`
//
// ### Released version
// - 4.7
//
// ### Backported versions
// - 4.6.20
//
// ### Notes
// None
func (g *Gatherer) GatherNetNamespace(ctx context.Context) ([]record.Record, []error) {
	gatherNetworkClient, err := networkv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherNetNamespace(ctx, gatherNetworkClient)
}

func gatherNetNamespace(ctx context.Context, networkClient networkv1client.NetworkV1Interface) ([]record.Record, []error) {
	nsList, err := networkClient.NetNamespaces().List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	var namespaces []*netNamespace
	for i := range nsList.Items {
		netNS := &netNamespace{
			Name:      nsList.Items[i].Name,
			EgressIPs: nsList.Items[i].EgressIPs,
			NetID:     nsList.Items[i].NetID,
		}
		namespaces = append(namespaces, netNS)
	}

	r := record.Record{
		Name: "config/netnamespaces",
		Item: record.JSONMarshaller{Object: namespaces},
	}

	return []record.Record{r}, nil
}
