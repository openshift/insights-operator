package tlsconfig

import (
	"crypto/tls"
	"fmt"

	"github.com/openshift/library-go/pkg/crypto"
)

// ServingInfo represents the TLS configuration extracted from the observed config.
// This is a simplified version of library-go's ServingInfo, containing only the fields
// we need for TLS configuration (minTLSVersion and cipherSuites).
//
// The full ServingInfo struct in library-go includes additional fields like certificates,
// bindAddress, etc., but we only care about TLS protocol settings here.
//
// This struct is designed to be extracted from the map[string]interface{} format
// returned by library-go's ObserveTLSSecurityProfile.
type ServingInfo struct {
	// MinTLSVersion is the minimum TLS version as a string (e.g., "VersionTLS12")
	// This matches the format used in Kubernetes ServingInfo configuration.
	MinTLSVersion string

	// CipherSuites is a list of cipher suite names in IANA format
	// (e.g., "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
	// These will be converted to Go's tls package constants (uint16).
	CipherSuites []string
}

// GetServingInfoFromObservedConfig extracts the ServingInfo from the observed config map
// returned by library-go's ObserveTLSSecurityProfile.
//
// The expected format in observedConfig is:
//
//	{
//	  "servingInfo": {
//	    "minTLSVersion": "VersionTLS12",
//	    "cipherSuites": ["TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", ...]
//	  }
//	}
func GetServingInfoFromObservedConfig(observedConfig map[string]interface{}) (*ServingInfo, error) {
	servingInfoRaw, exists := observedConfig["servingInfo"]
	if !exists {
		return nil, fmt.Errorf("servingInfo not found in observed config")
	}

	servingInfoMap, ok := servingInfoRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("servingInfo is not a map")
	}

	minTLSVersionRaw, exists := servingInfoMap["minTLSVersion"]
	if !exists {
		return nil, fmt.Errorf("minTLSVersion not found in servingInfo")
	}

	minTLSVersion, ok := minTLSVersionRaw.(string)
	if !ok {
		return nil, fmt.Errorf("minTLSVersion is not a string")
	}

	cipherSuitesRaw, exists := servingInfoMap["cipherSuites"]
	if !exists {
		return nil, fmt.Errorf("cipherSuites not found in servingInfo")
	}

	cipherSuitesSlice, ok := cipherSuitesRaw.([]interface{})
	if !ok {
		cipherSuitesStr, ok := cipherSuitesRaw.([]string)
		if !ok {
			return nil, fmt.Errorf("cipherSuites is not a slice")
		}
		return &ServingInfo{
			MinTLSVersion: minTLSVersion,
			CipherSuites:  cipherSuitesStr,
		}, nil
	}

	cipherSuites := make([]string, 0, len(cipherSuitesSlice))
	for _, cs := range cipherSuitesSlice {
		cipherStr, ok := cs.(string)
		if !ok {
			return nil, fmt.Errorf("cipher suite is not a string")
		}
		cipherSuites = append(cipherSuites, cipherStr)
	}

	return &ServingInfo{
		MinTLSVersion: minTLSVersion,
		CipherSuites:  cipherSuites,
	}, nil
}

// BuildTLSConfig converts a ServingInfo to a crypto/tls.Config.
//
// This function:
// - Uses library-go's crypto.TLSVersion() to parse the version string
// - Uses library-go's crypto.CipherSuite() to convert IANA cipher names to Go constants
// - Handles TLS 1.3 correctly (no CipherSuites field, as they are fixed in TLS 1.3)
func BuildTLSConfig(servingInfo *ServingInfo) (*tls.Config, error) {
	minVersion, err := crypto.TLSVersion(servingInfo.MinTLSVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid MinTLSVersion %q: %w", servingInfo.MinTLSVersion, err)
	}

	// nolint: gosec // G402: MinVersion is set from cluster TLS profile, not hardcoded
	config := &tls.Config{
		MinVersion: minVersion,
	}

	// TLS 1.3 cipher suites are fixed and cannot be configured
	if minVersion == tls.VersionTLS13 {
		config.MaxVersion = tls.VersionTLS13
	} else {
		cipherSuites := parseCipherSuitesFromIANA(servingInfo.CipherSuites)
		if len(cipherSuites) == 0 {
			return nil, fmt.Errorf("no valid cipher suites found")
		}
		config.CipherSuites = cipherSuites
	}

	return config, nil
}

// parseCipherSuitesFromIANA converts IANA cipher names to Go's tls constants
// using library-go's centralized mapping
func parseCipherSuitesFromIANA(ianaNames []string) []uint16 {
	suites := make([]uint16, 0, len(ianaNames))
	for _, name := range ianaNames {
		suite, err := crypto.CipherSuite(name)
		if err != nil {
			// Skip unknown ciphers for forward compatibility
			continue
		}
		suites = append(suites, suite)
	}
	return suites
}
