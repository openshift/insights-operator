package util

import (
	"context"
	"fmt"

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

// Clients bundles the typed clientsets, and facts about the cluster under
// test gathered once up front, needed by the integration tests.
type Clients struct {
	RestConfig *rest.Config
	Kube       kubernetes.Interface
	Insights   insightsclientset.Interface
	Config     configclientset.Interface

	// HasDefaultStorageClass reports whether the cluster has a default
	// StorageClass. It is determined once in Setup so individual specs
	// that need dynamically-provisioned storage can Skip cheaply instead
	// of each querying the API.
	HasDefaultStorageClass bool
}

// clients holds the result of Setup, for Get to return. Setup must be called
// once before any spec runs. It is wired through extension.AddBeforeAll in
// cmd/insights-operator-tests instead of a BeforeSuite.
var clients *Clients

// Setup builds the typed clientsets and gathers cluster facts (e.g.
// HasDefaultStorageClass) needed by the integration tests, and stores the
// result for Get to return. Call it once, via extension.AddBeforeAll.
func Setup(ctx context.Context) error {
	cfg, err := NewConfig()
	if err != nil {
		return fmt.Errorf("failed to load cluster config: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to build kube client: %w", err)
	}

	insightsClient, err := insightsclientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to build insights client: %w", err)
	}

	configClient, err := configclientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to build config client: %w", err)
	}

	hasDefaultStorageClass, err := HasDefaultStorageClass(ctx, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to determine default storage class: %w", err)
	}

	clients = &Clients{
		RestConfig:             cfg,
		Kube:                   kubeClient,
		Insights:               insightsClient,
		Config:                 configClient,
		HasDefaultStorageClass: hasDefaultStorageClass,
	}
	return nil
}

// Get returns the Clients built by Setup. It panics if called before Setup,
// since that is always a suite bootstrap bug (a missing AddBeforeAll call),
// not a runtime condition specs should handle.
func Get() *Clients {
	if clients == nil {
		panic("util.Get called before util.Setup (missing AddBeforeAll call?)")
	}
	return clients
}
