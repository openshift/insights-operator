package tlsconfig

import (
	"crypto/tls"
	"sync"

	"k8s.io/klog/v2"
)

// TLSConfigProvider provides TLS configuration from library-go observed config.
// nolint:revive // Name is intentionally explicit for clarity across packages
// It bridges the gap between library-go's configobserver pattern (which returns
// servingInfo format) and the HTTP clients that need crypto/tls.Config.
//
// This provider is thread-safe and can be called concurrently by multiple HTTP clients.
type TLSConfigProvider struct {
	mu             sync.RWMutex
	observedConfig map[string]interface{}
	tlsConfig      *tls.Config
}

// NewTLSConfigProvider creates a new TLS config provider from library-go observed config
func NewTLSConfigProvider() *TLSConfigProvider {
	return &TLSConfigProvider{
		observedConfig: make(map[string]interface{}),
	}
}

// UpdateObservedConfig updates the observed config and rebuilds the TLS config.
// This should be called whenever the config observer observes a change in the
// cluster's TLS security profile.
//
// The observedConfig should be in the servingInfo format returned by
// ObserveTLSSecurityProfile.
func (p *TLSConfigProvider) UpdateObservedConfig(observedConfig map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.observedConfig = observedConfig

	servingInfo, err := GetServingInfoFromObservedConfig(observedConfig)
	if err != nil {
		klog.Errorf("Failed to extract ServingInfo from observed config: %v", err)
		return err
	}

	tlsCfg, err := BuildTLSConfig(servingInfo)
	if err != nil {
		klog.Errorf("Failed to build TLS config from ServingInfo: %v", err)
		return err
	}

	p.tlsConfig = tlsCfg
	klog.Infof("Updated TLS config: minVersion=%x, ciphers=%d", tlsCfg.MinVersion, len(tlsCfg.CipherSuites))
	return nil
}

// GetTLSConfig returns the current TLS configuration.
// This method is thread-safe and implements the TLSConfigProvider interface
// expected by HTTP clients.
//
// The returned tls.Config is a clone of the cached config, so clients can
// safely modify it (e.g., to set RootCAs, ServerName, etc.).
//
// If the TLS config has not been initialized yet (no observed config has been
// set), this returns a safe default of TLS 1.2.
func (p *TLSConfigProvider) GetTLSConfig() *tls.Config {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.tlsConfig != nil {
		return p.tlsConfig.Clone()
	}

	klog.V(2).Info("TLS config not yet initialized, using default TLS 1.2")
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}
