package clusterconfig

import (
	"context"
	"testing"

	networkv1 "github.com/openshift/api/network/v1"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
)

func Test_GatherHostSubnet(t *testing.T) {
	testHostSubnet := networkv1.HostSubnet{
		Host:        "test.host",
		HostIP:      "10.0.0.0",
		Subnet:      "10.0.0.0/23",
		EgressIPs:   []networkv1.HostSubnetEgressIP{"10.0.0.0", "10.0.0.1"},
		EgressCIDRs: []networkv1.HostSubnetEgressCIDR{"10.0.0.0/24", "10.0.0.0/24"},
	}
	client := networkfake.NewSimpleClientset()
	_, err := client.NetworkV1().HostSubnets().Create(context.Background(), &testHostSubnet, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake hostsubnet")
	}

	ctx := context.Background()
	records, errs := gatherHostSubnet(ctx, client.NetworkV1())
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	_, err = records[0].Item.Marshal(context.TODO())
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}

	hs, ok := records[0].Item.(record.ResourceMarshaller).Resource.(*networkv1.HostSubnet)
	if !ok {
		t.Fatalf("failed to decode object")
	}
	if hs.HostIP != testHostSubnet.HostIP {
		t.Fatalf("Unexpected Host IP value %s", hs.HostIP)
	}
	if hs.Subnet != testHostSubnet.Subnet {
		t.Fatalf("Unexpected Subnet value %s", hs.Subnet)
	}
	if len(hs.EgressIPs) != len(testHostSubnet.EgressIPs) {
		t.Fatalf("unexpected number of egress IPs gathered %s", hs.EgressIPs)
	}

	if len(hs.EgressCIDRs) != len(testHostSubnet.EgressCIDRs) {
		t.Fatalf("unexpected number of egress CIDRs gathered %s", hs.EgressCIDRs)
	}
}
