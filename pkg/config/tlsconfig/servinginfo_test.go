package tlsconfig

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetServingInfoFromObservedConfig(t *testing.T) {
	tests := []struct {
		name            string
		observedConfig  map[string]interface{}
		expectError     bool
		expectedMinTLS  string
		expectedCiphers []string
	}{
		{
			name: "valid config with string slice",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "VersionTLS12",
					"cipherSuites": []string{
						"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
						"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
					},
				},
			},
			expectError:    false,
			expectedMinTLS: "VersionTLS12",
			expectedCiphers: []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
		},
		{
			name: "valid config with interface slice",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "VersionTLS13",
					"cipherSuites": []interface{}{
						"TLS_AES_128_GCM_SHA256",
						"TLS_AES_256_GCM_SHA384",
					},
				},
			},
			expectError:    false,
			expectedMinTLS: "VersionTLS13",
			expectedCiphers: []string{
				"TLS_AES_128_GCM_SHA256",
				"TLS_AES_256_GCM_SHA384",
			},
		},
		{
			name:           "missing servingInfo",
			observedConfig: map[string]interface{}{},
			expectError:    true,
		},
		{
			name: "servingInfo is not a map",
			observedConfig: map[string]interface{}{
				"servingInfo": "invalid",
			},
			expectError: true,
		},
		{
			name: "missing minTLSVersion",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"cipherSuites": []string{},
				},
			},
			expectError: true,
		},
		{
			name: "minTLSVersion is not a string",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": 12,
					"cipherSuites":  []string{},
				},
			},
			expectError: true,
		},
		{
			name: "missing cipherSuites",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "VersionTLS12",
				},
			},
			expectError: true,
		},
		{
			name: "cipherSuites is not a slice",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "VersionTLS12",
					"cipherSuites":  "invalid",
				},
			},
			expectError: true,
		},
		{
			name: "cipher suite item is not a string",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "VersionTLS12",
					"cipherSuites": []interface{}{
						"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
						123,
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servingInfo, err := GetServingInfoFromObservedConfig(tt.observedConfig)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, servingInfo)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, servingInfo)
				assert.Equal(t, tt.expectedMinTLS, servingInfo.MinTLSVersion)
				assert.Equal(t, tt.expectedCiphers, servingInfo.CipherSuites)
			}
		})
	}
}

func TestBuildTLSConfig(t *testing.T) {
	tests := []struct {
		name              string
		servingInfo       *ServingInfo
		expectError       bool
		expectedMinVer    uint16
		expectedMaxVer    uint16
		expectedCipherLen int
	}{
		{
			name: "TLS 1.2 with ciphers",
			servingInfo: &ServingInfo{
				MinTLSVersion: "VersionTLS12",
				CipherSuites: []string{
					"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
					"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
				},
			},
			expectError:       false,
			expectedMinVer:    tls.VersionTLS12,
			expectedMaxVer:    0, // Not set for TLS 1.2
			expectedCipherLen: 2,
		},
		{
			name: "TLS 1.3 with fixed ciphers",
			servingInfo: &ServingInfo{
				MinTLSVersion: "VersionTLS13",
				CipherSuites:  []string{}, // TLS 1.3 ciphers are fixed
			},
			expectError:       false,
			expectedMinVer:    tls.VersionTLS13,
			expectedMaxVer:    tls.VersionTLS13,
			expectedCipherLen: 0, // CipherSuites should not be set for TLS 1.3
		},
		{
			name: "invalid TLS version",
			servingInfo: &ServingInfo{
				MinTLSVersion: "InvalidVersion",
				CipherSuites:  []string{},
			},
			expectError: true,
		},
		{
			name: "no valid cipher suites",
			servingInfo: &ServingInfo{
				MinTLSVersion: "VersionTLS12",
				CipherSuites: []string{
					"INVALID_CIPHER_1",
					"INVALID_CIPHER_2",
				},
			},
			expectError: true,
		},
		{
			name: "mixed valid and invalid ciphers",
			servingInfo: &ServingInfo{
				MinTLSVersion: "VersionTLS12",
				CipherSuites: []string{
					"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
					"INVALID_CIPHER",
					"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
				},
			},
			expectError:       false,
			expectedMinVer:    tls.VersionTLS12,
			expectedCipherLen: 2, // Invalid cipher should be skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfig, err := BuildTLSConfig(tt.servingInfo)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, tlsConfig)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tlsConfig)
				assert.Equal(t, tt.expectedMinVer, tlsConfig.MinVersion)
				assert.Equal(t, tt.expectedMaxVer, tlsConfig.MaxVersion)
				assert.Len(t, tlsConfig.CipherSuites, tt.expectedCipherLen)
			}
		})
	}
}

func TestParseCipherSuitesFromIANA(t *testing.T) {
	tests := []struct {
		name          string
		ianaNames     []string
		expectedLen   int
		expectedFirst uint16
	}{
		{
			name: "valid IANA cipher names",
			ianaNames: []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
			expectedLen:   2,
			expectedFirst: tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		{
			name: "invalid cipher names are skipped",
			ianaNames: []string{
				"INVALID_CIPHER_1",
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"INVALID_CIPHER_2",
			},
			expectedLen:   1,
			expectedFirst: tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		{
			name:        "empty input",
			ianaNames:   []string{},
			expectedLen: 0,
		},
		{
			name: "all invalid ciphers",
			ianaNames: []string{
				"INVALID_1",
				"INVALID_2",
			},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suites := parseCipherSuitesFromIANA(tt.ianaNames)
			assert.Len(t, suites, tt.expectedLen)
			if tt.expectedLen > 0 {
				assert.Equal(t, tt.expectedFirst, suites[0])
			}
		})
	}
}
