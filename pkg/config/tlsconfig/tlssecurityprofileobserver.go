package tlsconfig

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/crypto"
	"github.com/openshift/library-go/pkg/operator/configobserver"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	tlsMinVersionPath = "servingInfo.minTLSVersion"
	tlsCiphersPath    = "servingInfo.cipherSuites"
)

// TLSProfileListers holds the listers needed for TLS profile observation
type TLSProfileListers struct {
	apiserverLister configlistersv1.APIServerLister
	resourceSyncer  resourcesynccontroller.ResourceSyncer
	informers       []cache.InformerSynced
}

// NewTLSProfileListers creates a new TLSProfileListers instance
func NewTLSProfileListers(
	apiServerLister configlistersv1.APIServerLister,
	resourceSyncer resourcesynccontroller.ResourceSyncer,
	informers []cache.InformerSynced,
) *TLSProfileListers {
	return &TLSProfileListers{
		apiserverLister: apiServerLister,
		resourceSyncer:  resourceSyncer,
		informers:       informers,
	}
}

func (l *TLSProfileListers) APIServerLister() configlistersv1.APIServerLister {
	return l.apiserverLister
}

func (l *TLSProfileListers) ResourceSyncer() resourcesynccontroller.ResourceSyncer {
	return l.resourceSyncer
}

func (l *TLSProfileListers) PreRunHasSynced() []cache.InformerSynced {
	return l.informers
}

// ObserveTLSSecurityProfile observes the APIServer TLS security profile and returns the
// TLS configuration in servingInfo format (minTLSVersion and cipherSuites).
//
// This function follows the library-go configobserver pattern and centralizes TLS profile
// handling as recommended by the OpenShift platform guidelines.
//
// The returned configuration is in the format:
//
//	servingInfo:
//	  minTLSVersion: "VersionTLS12"
//	  cipherSuites: ["TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", ...]
func ObserveTLSSecurityProfile(
	listers configobserver.Listers,
	recorder events.Recorder,
	existingConfig map[string]interface{},
) (map[string]interface{}, []error) {
	defer func() {
		if r := recover(); r != nil {
			recorder.Warningf("ObserveTLSSecurityProfileFailed", "Panic while observing TLS profile: %v", r)
		}
	}()

	errs := []error{}

	tlsListers, ok := listers.(*TLSProfileListers)
	if !ok {
		// Can't observe without proper listers, preserve existing config
		return existingConfig, append(errs, fmt.Errorf("invalid listers type: expected TLSProfileListers"))
	}

	apiServer, err := tlsListers.APIServerLister().Get("cluster")
	if errors.IsNotFound(err) {
		// APIServer doesn't exist, use default Intermediate profile
		klog.V(2).Info("APIServer 'cluster' not found, using default Intermediate TLS profile")
		return observeTLSProfile(nil, existingConfig, recorder, errs)
	}
	if err != nil {
		// Preserve existing config on error
		recorder.Warningf("ObserveTLSSecurityProfileFailed", "Failed to get APIServer: %v", err)
		return existingConfig, append(errs, fmt.Errorf("failed to get APIServer: %w", err))
	}

	return observeTLSProfile(apiServer.Spec.TLSSecurityProfile, existingConfig, recorder, errs)
}

func observeTLSProfile(
	profile *configv1.TLSSecurityProfile,
	existingConfig map[string]interface{},
	recorder events.Recorder,
	errs []error,
) (map[string]interface{}, []error) {
	observedConfig := map[string]interface{}{}

	profileSpec, err := getTLSProfileSpec(profile)
	if err != nil {
		recorder.Warningf("ObserveTLSSecurityProfileFailed", "Failed to get TLS profile spec: %v", err)
		return existingConfig, append(errs, err)
	}

	minVersion := string(profileSpec.MinTLSVersion)
	if err := setObservedField(observedConfig, tlsMinVersionPath, minVersion); err != nil {
		recorder.Warningf("ObserveTLSSecurityProfileFailed", "Failed to set minTLSVersion: %v", err)
		return existingConfig, append(errs, err)
	}

	ianaCiphers := crypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers)

	if err := setObservedField(observedConfig, tlsCiphersPath, ianaCiphers); err != nil {
		recorder.Warningf("ObserveTLSSecurityProfileFailed", "Failed to set cipherSuites: %v", err)
		return existingConfig, append(errs, err)
	}

	recorder.Eventf("ObserveTLSSecurityProfile", "Observed TLS profile: type=%s, minVersion=%s, ciphers=%d",
		getProfileTypeName(profile), minVersion, len(ianaCiphers))

	return observedConfig, errs
}

func getTLSProfileSpec(profile *configv1.TLSSecurityProfile) (*configv1.TLSProfileSpec, error) {
	// If no profile specified, use Intermediate
	if profile == nil {
		return configv1.TLSProfiles[configv1.TLSProfileIntermediateType], nil
	}

	switch profile.Type {
	case configv1.TLSProfileOldType,
		configv1.TLSProfileIntermediateType,
		configv1.TLSProfileModernType:
		spec := configv1.TLSProfiles[profile.Type]
		if spec == nil {
			return nil, fmt.Errorf("built-in TLS profile %s not found", profile.Type)
		}
		return spec, nil
	case configv1.TLSProfileCustomType:
		if profile.Custom == nil {
			return nil, fmt.Errorf("custom TLS profile specified but Custom field is nil")
		}
		return &profile.Custom.TLSProfileSpec, nil
	default:
		klog.Warningf("Unknown TLS profile type %s, using Intermediate", profile.Type)
		return configv1.TLSProfiles[configv1.TLSProfileIntermediateType], nil
	}
}

func getProfileTypeName(profile *configv1.TLSSecurityProfile) string {
	if profile == nil {
		return string(configv1.TLSProfileIntermediateType)
	}
	return string(profile.Type)
}

// setObservedField sets a field in the observed config using dot notation path
func setObservedField(config map[string]interface{}, path string, value interface{}) error {
	parts := []string{}
	current := ""
	for _, c := range path {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	if len(parts) == 0 {
		return fmt.Errorf("empty path")
	}

	curr := config
	for i := 0; i < len(parts)-1; i++ {
		next, exists := curr[parts[i]]
		if !exists {
			next = map[string]interface{}{}
			curr[parts[i]] = next
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("path %s is not a map at level %s", path, parts[i])
		}
		curr = nextMap
	}

	curr[parts[len(parts)-1]] = value
	return nil
}
