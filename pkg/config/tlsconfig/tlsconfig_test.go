package tlsconfig

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
)

func TestBuildTLSConfigFromProfile_Intermediate(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileIntermediateType,
	}

	config, err := BuildTLSConfigFromProfile(profile)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS12, int(config.MinVersion))
	assert.NotEmpty(t, config.CipherSuites)
}

func TestBuildTLSConfigFromProfile_Modern(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileModernType,
	}

	config, err := BuildTLSConfigFromProfile(profile)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS13, int(config.MinVersion))
	assert.Equal(t, tls.VersionTLS13, int(config.MaxVersion))
	// TLS 1.3 should not have CipherSuites set
	assert.Empty(t, config.CipherSuites)
}

func TestBuildTLSConfigFromProfile_Old(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileOldType,
	}

	config, err := BuildTLSConfigFromProfile(profile)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS10, int(config.MinVersion))
	assert.NotEmpty(t, config.CipherSuites)
}

func TestBuildTLSConfigFromProfile_Custom(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers: []string{
					"ECDHE-RSA-AES128-GCM-SHA256",
					"ECDHE-RSA-AES256-GCM-SHA384",
				},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}

	config, err := BuildTLSConfigFromProfile(profile)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS12, int(config.MinVersion))
	assert.Len(t, config.CipherSuites, 2)
	assert.Contains(t, config.CipherSuites, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
	assert.Contains(t, config.CipherSuites, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384)
}

func TestBuildTLSConfigFromProfile_Nil(t *testing.T) {
	// Nil profile should default to Intermediate
	config, err := BuildTLSConfigFromProfile(nil)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS12, int(config.MinVersion))
	assert.NotEmpty(t, config.CipherSuites)
}

func TestBuildTLSConfigFromProfile_CustomNilSpec(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type:   configv1.TLSProfileCustomType,
		Custom: nil, // Invalid: Custom type but nil Custom field
	}

	_, err := BuildTLSConfigFromProfile(profile)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "custom profile specified but Custom field is nil")
}

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  configv1.TLSProtocolVersion
		expected uint16
		wantErr  bool
	}{
		{
			name:     "TLS 1.0",
			version:  configv1.VersionTLS10,
			expected: tls.VersionTLS10,
			wantErr:  false,
		},
		{
			name:     "TLS 1.1",
			version:  configv1.VersionTLS11,
			expected: tls.VersionTLS11,
			wantErr:  false,
		},
		{
			name:     "TLS 1.2",
			version:  configv1.VersionTLS12,
			expected: tls.VersionTLS12,
			wantErr:  false,
		},
		{
			name:     "TLS 1.3",
			version:  configv1.VersionTLS13,
			expected: tls.VersionTLS13,
			wantErr:  false,
		},
		{
			name:     "Invalid version",
			version:  "VersionTLS99",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTLSVersion(tt.version)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseOpenSSLCipherSuites(t *testing.T) {
	tests := []struct {
		name          string
		opensslNames  []string
		expectedCount int
		wantErr       bool
	}{
		{
			name: "Valid TLS 1.2 ciphers",
			opensslNames: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"ECDHE-RSA-AES256-GCM-SHA384",
			},
			expectedCount: 2,
			wantErr:       false,
		},
		{
			name: "Mixed valid and invalid ciphers",
			opensslNames: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"UNKNOWN-CIPHER-SUITE",
				"ECDHE-RSA-AES256-GCM-SHA384",
			},
			expectedCount: 2, // Only valid ones
			wantErr:       false,
		},
		{
			name: "All invalid ciphers",
			opensslNames: []string{
				"UNKNOWN-CIPHER-1",
				"UNKNOWN-CIPHER-2",
			},
			expectedCount: 0,
			wantErr:       true,
		},
		{
			name:          "Empty cipher list",
			opensslNames:  []string{},
			expectedCount: 0,
			wantErr:       true,
		},
		{
			name: "Legacy cipher (3DES)",
			opensslNames: []string{
				"DES-CBC3-SHA",
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "CHACHA20 ciphers",
			opensslNames: []string{
				"ECDHE-ECDSA-CHACHA20-POLY1305",
				"ECDHE-RSA-CHACHA20-POLY1305",
			},
			expectedCount: 2,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseOpenSSLCipherSuites(tt.opensslNames)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
			}
		})
	}
}

func TestGetTLSProfileSpec(t *testing.T) {
	tests := []struct {
		name            string
		profile         *configv1.TLSSecurityProfile
		expectedVersion configv1.TLSProtocolVersion
		wantErr         bool
	}{
		{
			name:            "Nil profile defaults to Intermediate",
			profile:         nil,
			expectedVersion: configv1.VersionTLS12,
			wantErr:         false,
		},
		{
			name: "Old profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			expectedVersion: configv1.VersionTLS10,
			wantErr:         false,
		},
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectedVersion: configv1.VersionTLS12,
			wantErr:         false,
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectedVersion: configv1.VersionTLS13,
			wantErr:         false,
		},
		{
			name: "Custom profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
			expectedVersion: configv1.VersionTLS12,
			wantErr:         false,
		},
		{
			name: "Unknown profile type falls back to Intermediate",
			profile: &configv1.TLSSecurityProfile{
				Type: "UnknownType",
			},
			expectedVersion: configv1.VersionTLS12,
			wantErr:         false,
		},
		{
			name: "Custom profile with nil Custom field",
			profile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: nil,
			},
			expectedVersion: "",
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getTLSProfileSpec(tt.profile)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedVersion, result.MinTLSVersion)
			}
		})
	}
}

func TestTLSConfigProvider_GetTLSConfig_NilClient(t *testing.T) {
	provider := NewTLSConfigProvider(nil)

	config := provider.GetTLSConfig()

	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS12, int(config.MinVersion))
}

func TestCipherMapping_IntermediateProfile(t *testing.T) {
	// Verify that Intermediate profile ciphers are properly mapped
	intermediateSpec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	cipherSuites, err := parseOpenSSLCipherSuites(intermediateSpec.Ciphers)

	assert.NoError(t, err)
	assert.NotEmpty(t, cipherSuites)

	// Verify some expected ciphers are present
	assert.Contains(t, cipherSuites, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
	assert.Contains(t, cipherSuites, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
}

func TestCipherMapping_OldProfile(t *testing.T) {
	// Verify that Old profile ciphers (including legacy ones) are properly mapped
	oldSpec := configv1.TLSProfiles[configv1.TLSProfileOldType]

	cipherSuites, err := parseOpenSSLCipherSuites(oldSpec.Ciphers)

	assert.NoError(t, err)
	assert.NotEmpty(t, cipherSuites)

	// Old profile should include 3DES
	assert.Contains(t, cipherSuites, tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA)
}

func TestBuildTLSConfig_TLS13_NoCipherSuites(t *testing.T) {
	// TLS 1.3 should not have CipherSuites field set
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS13,
				Ciphers: []string{
					// Even if ciphers are specified, they should be ignored for TLS 1.3
					"TLS_AES_128_GCM_SHA256",
					"TLS_AES_256_GCM_SHA384",
				},
			},
		},
	}

	config, err := BuildTLSConfigFromProfile(profile)

	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, tls.VersionTLS13, int(config.MinVersion))
	assert.Equal(t, tls.VersionTLS13, int(config.MaxVersion))
	// CipherSuites should be empty for TLS 1.3
	assert.Empty(t, config.CipherSuites)
}
