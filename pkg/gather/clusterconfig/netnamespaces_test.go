package clusterconfig

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	networkv1 "github.com/openshift/api/network/v1"
	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_NetNamespaces_Gather(t *testing.T) {
	ns1 := &networkv1.NetNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespaces-1",
		},
		EgressIPs: []networkv1.NetNamespaceEgressIP{"10.10.10.10"},
		NetID:     12345,
	}
	ns2 := &networkv1.NetNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespaces-2",
		},
		EgressIPs: []networkv1.NetNamespaceEgressIP{"11.11.11.11"},
		NetID:     67891,
	}
	ctx := context.Background()
	cs := networkfake.NewSimpleClientset()
	createNetNamespaces(ctx, t, cs, ns1, ns2)
	rec, errs := gatherNetNamespace(ctx, cs.NetworkV1())
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(rec) != 1 {
		t.Fatalf("unexpected number or records %d", len(rec))
	}
	it1 := rec[0].Item
	it1Bytes, err := it1.Marshal(context.TODO())
	if err != nil {
		t.Fatalf("unable to marshal: %v", err)
	}
	var netNamespaces []netNamespace
	err = json.Unmarshal(it1Bytes, &netNamespaces)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if len(netNamespaces) != 2 {
		t.Fatalf("unexpected number of namespaces gathered %d", len(netNamespaces))
	}
	if !equalNetNamespaceS(ns1, netNamespaces[0]) {
		t.Fatalf("unexpected netnamespace %v ", netNamespaces[0])
	}

	if !equalNetNamespaceS(ns2, netNamespaces[1]) {
		t.Fatalf("unexpected netnamespace %v ", netNamespaces[1])
	}
}

func equalNetNamespaceS(ns1 *networkv1.NetNamespace, ns2 netNamespace) bool {
	if ns1.Name != ns2.Name {
		return false
	}
	if ns1.NetID != ns2.NetID {
		return false
	}
	if !reflect.DeepEqual(ns1.EgressIPs, ns2.EgressIPs) {
		return false
	}
	return true
}

func createNetNamespaces(ctx context.Context, t *testing.T, n *networkfake.Clientset, namespaces ...*networkv1.NetNamespace) {
	for _, ns := range namespaces {
		_, err := n.NetworkV1().NetNamespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake netnamespace", err)
		}
	}
}
