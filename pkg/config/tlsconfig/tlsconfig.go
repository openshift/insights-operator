package tlsconfig

import (
	"context"
	"crypto/tls"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// TLSConfigProvider provides TLS configuration based on cluster TLS security profile
type TLSConfigProvider struct {
	configClient configv1client.ConfigV1Interface
}

// NewTLSConfigProvider creates a new TLS config provider
func NewTLSConfigProvider(configClient configv1client.ConfigV1Interface) *TLSConfigProvider {
	return &TLSConfigProvider{
		configClient: configClient,
	}
}

// GetTLSConfig fetches the cluster TLS profile and returns a tls.Config
func (p *TLSConfigProvider) GetTLSConfig() *tls.Config {
	if p.configClient == nil {
		klog.V(4).Info("TLS config client not available, using default TLS 1.2")
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}

	ctx := context.Background()
	apiServer, err := p.configClient.APIServers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		klog.V(4).Infof("Failed to get APIServer resource, using default TLS 1.2: %v", err)
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}

	profile := apiServer.Spec.TLSSecurityProfile
	tlsConfig, err := BuildTLSConfigFromProfile(profile)
	if err != nil {
		klog.Warningf("Failed to build TLS config from profile, using default TLS 1.2: %v", err)
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return tlsConfig
}

// BuildTLSConfigFromProfile converts a TLS security profile to crypto/tls.Config
func BuildTLSConfigFromProfile(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	profileSpec, err := getTLSProfileSpec(profile)
	if err != nil {
		return nil, err
	}

	minVersion, err := parseTLSVersion(profileSpec.MinTLSVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid min TLS version %q: %w", profileSpec.MinTLSVersion, err)
	}

	// #nosec G402 - MinVersion is set from cluster TLS profile, which may be TLS 1.0/1.1 for Old profile
	config := &tls.Config{
		MinVersion: minVersion,
	}

	// TLS 1.3 uses fixed cipher suites, don't set CipherSuites field
	if minVersion == tls.VersionTLS13 {
		config.MaxVersion = tls.VersionTLS13
		return config, nil
	}

	if len(profileSpec.Ciphers) > 0 {
		cipherSuites, err := parseOpenSSLCipherSuites(profileSpec.Ciphers)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cipher suites: %w", err)
		}
		config.CipherSuites = cipherSuites
	}

	return config, nil
}

// getTLSProfileSpec returns the TLSProfileSpec for the given profile
func getTLSProfileSpec(profile *configv1.TLSSecurityProfile) (*configv1.TLSProfileSpec, error) {
	if profile == nil {
		return configv1.TLSProfiles[configv1.TLSProfileIntermediateType], nil
	}

	switch profile.Type {
	case configv1.TLSProfileOldType,
		configv1.TLSProfileIntermediateType,
		configv1.TLSProfileModernType:
		return configv1.TLSProfiles[profile.Type], nil
	case configv1.TLSProfileCustomType:
		if profile.Custom == nil {
			return nil, fmt.Errorf("custom profile specified but Custom field is nil")
		}
		return &profile.Custom.TLSProfileSpec, nil
	default:
		klog.Warningf("Unknown TLS profile type %q, falling back to Intermediate", profile.Type)
		return configv1.TLSProfiles[configv1.TLSProfileIntermediateType], nil
	}
}

// parseTLSVersion converts a TLS version string to uint16
func parseTLSVersion(version configv1.TLSProtocolVersion) (uint16, error) {
	switch version {
	case configv1.VersionTLS10:
		return tls.VersionTLS10, nil
	case configv1.VersionTLS11:
		return tls.VersionTLS11, nil
	case configv1.VersionTLS12:
		return tls.VersionTLS12, nil
	case configv1.VersionTLS13:
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unknown TLS version: %s", version)
	}
}

// parseOpenSSLCipherSuites converts OpenSSL cipher suite names to Go constants
func parseOpenSSLCipherSuites(opensslNames []string) ([]uint16, error) {
	// Map OpenSSL cipher names to Go's tls package constants
	// Based on: https://pkg.go.dev/crypto/tls#pkg-constants
	// Only includes cipher suites that are actually defined in Go's crypto/tls package
	cipherMap := map[string]uint16{
		// TLS 1.3 cipher suites
		"TLS_AES_128_GCM_SHA256":       tls.TLS_AES_128_GCM_SHA256,
		"TLS_AES_256_GCM_SHA384":       tls.TLS_AES_256_GCM_SHA384,
		"TLS_CHACHA20_POLY1305_SHA256": tls.TLS_CHACHA20_POLY1305_SHA256,

		// TLS 1.2 ECDHE GCM cipher suites
		"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

		// TLS 1.2 ECDHE CBC cipher suites
		"ECDHE-ECDSA-AES128-SHA": tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		"ECDHE-RSA-AES128-SHA":   tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		"ECDHE-ECDSA-AES256-SHA": tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		"ECDHE-RSA-AES256-SHA":   tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,

		// TLS 1.2 RSA cipher suites
		"AES128-GCM-SHA256": tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"AES256-GCM-SHA384": tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		"AES128-SHA":        tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"AES256-SHA":        tls.TLS_RSA_WITH_AES_256_CBC_SHA,

		// Legacy cipher suites (for Old profile)
		"DES-CBC3-SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		"ECDHE-RSA-DES-CBC3-SHA": tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	}

	var suites []uint16
	for _, name := range opensslNames {
		if suite, ok := cipherMap[name]; ok {
			suites = append(suites, suite)
		} else {
			klog.V(4).Infof("Unknown cipher suite %q, skipping", name)
		}
	}

	if len(suites) == 0 {
		return nil, fmt.Errorf("no valid cipher suites found")
	}

	return suites, nil
}
