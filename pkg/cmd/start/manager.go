package start

import (
	"context"
	"crypto/tls"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	utiltls "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/openshift/insights-operator/pkg/insights"
	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/insights/insightsreport"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	healthProbeBindAddress  = ":8080"
	metricsBindAddress      = ":8443"
	serviceCACertSecretPath = "/var/run/secrets/serving-cert"
)

// NewManager initializes and configures a controller-runtime manager that provides:
// - Secure metrics endpoint on :8443 with TLS from cluster APIServer profile
// - Health check endpoints (healthz on :8080, readyz on :8080)
// - TLS security profile watcher that triggers restart on TLS configuration changes
//
// The function fetches the current TLS profile and adherence policy from the cluster's
// APIServer resource, applies it to the metrics server, and sets up a watcher to detect
// any changes. When TLS settings change, the provided cancelFunc is called to trigger
// operator restart.
func NewManager(
	ctx context.Context,
	clientConfig *rest.Config,
	scheme *runtime.Scheme,
	namespace string,
	cancelFunc context.CancelFunc,
) (manager.Manager, error) {
	// Set logger for controller-runtime
	controllerruntime.SetLogger(klog.NewKlogr())

	// Register insights-operator metrics with controller-runtime's registry
	if err := insights.RegisterInsightsMetrics(metrics.Registry); err != nil {
		return nil, fmt.Errorf("failed to register insights metrics: %v", err)
	}
	if err := metrics.Registry.Register(insightsclient.GetRecvReportMetric()); err != nil {
		return nil, fmt.Errorf("failed to register recv report metric: %v", err)
	}
	if err := metrics.Registry.Register(insightsreport.GetInsightsStatusMetric()); err != nil {
		return nil, fmt.Errorf("failed to register insights status metric: %v", err)
	}

	k8sClient, err := client.New(clientConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Fetch the TLS profile from the APIServer resource.
	tlsSecurityProfileSpec, err := utiltls.FetchAPIServerTLSProfile(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("unable to get TLS profile from API server: %v", err)
	}

	// Fetch the TLS adherence policy from the APIServer resource
	tlsAdherencePolicy, err := utiltls.FetchAPIServerTLSAdherencePolicy(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("unable to get TLS adherence policy from API server: %v", err)
	}

	// Create the TLS configuration function for the server endpoints.
	tlsConfigFn, unsupportedCiphers := utiltls.NewTLSConfigFromProfile(tlsSecurityProfileSpec)
	if len(unsupportedCiphers) > 0 {
		klog.Infof("TLS configuration contains unsupported ciphers that will be ignored: %v", unsupportedCiphers)
	}

	mgr, err := createManager(clientConfig, tlsConfigFn, scheme, namespace)
	if err != nil {
		return nil, err
	}

	// Set up TLS security profile watcher to trigger shutdown on TLS changes
	tlsWatcher := &utiltls.SecurityProfileWatcher{
		Client:                    mgr.GetClient(),
		InitialTLSProfileSpec:     tlsSecurityProfileSpec,
		InitialTLSAdherencePolicy: tlsAdherencePolicy,
		OnProfileChange: func(ctx context.Context, oldTLSProfileSpec, newTLSProfileSpec configv1.TLSProfileSpec) {
			klog.Infof("TLS profile has changed, initiating a shutdown to reload it. %q: %+v, %q: %+v",
				"old profile", oldTLSProfileSpec,
				"new profile", newTLSProfileSpec,
			)
			cancelFunc()
		},
		OnAdherencePolicyChange: func(ctx context.Context, oldTLSAdherencePolicy, newTLSAdherencePolicy configv1.TLSAdherencePolicy) {
			klog.Infof("tlsAdherencePolicy has changed, initiating a shutdown to reload it. %q: %+v, %q: %+v",
				"old tlsAdherencePolicy", oldTLSAdherencePolicy,
				"new tlsAdherencePolicy", newTLSAdherencePolicy,
			)
			cancelFunc()
		},
	}

	// Register watcher with manager
	if err := tlsWatcher.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("unable to set up TLS security profile watcher: %v", err)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up healthz endpoint: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up readyz endpoint: %v", err)
	}

	return mgr, nil
}

// createManager creates a controller-runtime manager configured for serving metrics.
//
// The manager is configured with:
//   - Secure metrics server on :8443 with TLS and authentication/authorization
//   - Health probe endpoint on :8080
//   - Leader election disabled (metrics don't require leader election)
func createManager(
	clientConfig *rest.Config,
	tlsConfigFn func(*tls.Config),
	scheme *runtime.Scheme,
	namespace string,
) (manager.Manager, error) {
	return manager.New(clientConfig, manager.Options{
		Metrics: server.Options{
			BindAddress:   metricsBindAddress,
			SecureServing: true,
			// CertDir points to the service-ca certificate mounted by the deployment
			CertDir: serviceCACertSecretPath,
			// FilterProvider handles client authentication and authorization
			FilterProvider: filters.WithAuthenticationAndAuthorization,
			// TLSOpts handles server TLS configuration (from cluster APIServer TLS profile)
			TLSOpts: []func(*tls.Config){
				tlsConfigFn,
			},
		},
		LeaderElectionNamespace:       namespace,
		LeaderElection:                false,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
		Scheme:                        scheme,
		HealthProbeBindAddress:        healthProbeBindAddress,
	})
}
