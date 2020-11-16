package clusterconfig

import (
	"context"
	"testing"

	networkv1 "github.com/openshift/api/network/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
)

func TestGatherHostSubnet(t *testing.T) {
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
	item, err := records[0].Item.Marshal(context.TODO())
	var gatheredHostSubnet networkv1.HostSubnet
	_, _, err = networkSerializer.LegacyCodec(networkv1.SchemeGroupVersion).Decode(item, nil, &gatheredHostSubnet)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredHostSubnet.HostIP != "xxxxxxxx" {
		t.Fatalf("Host IP is not anonymized %s", gatheredHostSubnet.HostIP)
	}
	if gatheredHostSubnet.Subnet != "xxxxxxxxxxx" {
		t.Fatalf("Host Subnet is not anonymized %s", gatheredHostSubnet.Subnet)
	}
	if len(gatheredHostSubnet.EgressIPs) != len(testHostSubnet.EgressIPs) {
		t.Fatalf("unexpected number of egress IPs gathered %s", gatheredHostSubnet.EgressIPs)
	}

	if len(gatheredHostSubnet.EgressCIDRs) != len(testHostSubnet.EgressCIDRs) {
		t.Fatalf("unexpected number of egress CIDRs gathered %s", gatheredHostSubnet.EgressCIDRs)
	}

	for _, ip := range gatheredHostSubnet.EgressIPs {
		if ip != "xxxxxxxx" {
			t.Fatalf("Egress IP is not anonymized %s", ip)
		}
	}

	for _, cidr := range gatheredHostSubnet.EgressCIDRs {
		if cidr != "xxxxxxxxxxx" {
			t.Fatalf("Egress CIDR is not anonymized %s", cidr)
		}
	}
}
