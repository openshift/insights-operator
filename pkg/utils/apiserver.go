package utils

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetClusterAPIServerInfo returns the cluster API server URL
// In Hypershift API Server and internal API Server are not required to be in a subdomain of the base domain.
//
// For example, given the base domain `openshift.example.com`,
// an API server could be in `api.hypershift.local`.
//
// It uses this API https://docs.openshift.com/container-platform/4.7/rest_api/config_apis/dns-config-openshift-io-v1.html
func GetClusterAPIServerInfo(ctx context.Context, configClient configv1client.ConfigV1Interface) ([]string, error) {
	infra, err := configClient.Infrastructures().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return []string{}, err
	}

	urls := []string{}
	for _, url := range []string{infra.Status.APIServerURL, infra.Status.APIServerInternalURL} {
		if url != "" {
			urls = append(urls, url)
		}
	}

	return urls, nil
}
