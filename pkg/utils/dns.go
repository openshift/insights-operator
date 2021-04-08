package utils

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetClusterBaseDomain returns a base domain for the cluster.
// Base domain is the base domain of the cluster. All managed DNS records will
// be sub-domains of this base.
//
// For example, given the base domain `openshift.example.com`, an API server
// DNS record may be created for `cluster-api.openshift.example.com`.
//
// It uses this API https://docs.openshift.com/container-platform/4.7/rest_api/config_apis/dns-config-openshift-io-v1.html
func GetClusterBaseDomain(ctx context.Context, configClient configv1client.ConfigV1Interface) (string, error) {
	dns, err := configClient.DNSes().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	return dns.Spec.BaseDomain, nil
}
