package clusterconfig

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/openshift/api/network/v1"
	networkv1client "github.com/openshift/client-go/network/clientset/versioned/typed/network/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

type netNamespace struct {
	Name      string                           `json:"name"`
	EgressIPs []networkv1.NetNamespaceEgressIP `json:"egressIPs"`
	NetID     uint32                           `json:"netID"`
}

// GatherNetNamespace collects NetNamespaces networking information
//
// The Kubernetes api https://github.com/openshift/client-go/blob/master/network/clientset/versioned/typed/network/v1/netnamespace.go
// Response is an array of netNamespaces. Netnamespace contains Name, EgressIPs and NetID attributes.
//
// Location in archive: config/netnamespaces
// Id in config: netnamespaces
func GatherNetNamespace(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		nsList, err := g.networkClient.NetNamespaces().List(g.ctx, metav1.ListOptions{})
		if errors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, []error{err}
		}
		namespaces := []*netNamespace{}
		for _, n := range nsList.Items {
			netNS := &netNamespace{
				Name:      n.Name,
				EgressIPs: n.EgressIPs,
				NetID:     n.NetID,
			}
			namespaces = append(namespaces, netNS)
		}
		r := record.Record{
			Name: "config/netnamespaces",
			Item: NetNamespaceAnonymizer{namespaces: namespaces},
		}
		return []record.Record{r}, nil
	}
}


// NetNamespaceAnonymizer implements NetNamespace serialization
type NetNamespaceAnonymizer struct{ namespaces []*netNamespace }

// Marshal implements NetNamespace serialization
func (a NetNamespaceAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(a.namespaces)
}

// GetExtension returns extension for NetNamespace object
func (a NetNamespaceAnonymizer) GetExtension() string {
	return "json"
}
