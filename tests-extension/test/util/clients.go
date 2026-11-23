package util

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	configclientset "github.com/openshift/client-go/config/clientset/versioned"
	insightsclientset "github.com/openshift/client-go/insights/clientset/versioned"
)

// NewConfig returns the *rest.Config for the cluster under test, using the
// standard KUBECONFIG resolution (KUBECONFIG env var, then
// $HOME/.kube/config, then in-cluster config).
func NewConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// Clients bundles the typed clientsets needed by the integration tests.
type Clients struct {
	Kube     kubernetes.Interface
	Insights insightsclientset.Interface
	Config   configclientset.Interface
}

// NewClients builds the typed clientsets needed by the integration tests from
// the given rest.Config.
func NewClients(cfg *rest.Config) (*Clients, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	insightsClient, err := insightsclientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	configClient, err := configclientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Clients{Kube: kubeClient, Insights: insightsClient, Config: configClient}, nil
}
