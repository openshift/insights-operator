package insightsclient

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
)

func Test_parseTLSVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expected    uint16
		expectError bool
	}{
		{
			name:        "TLS 1.0 with TLSv1.0",
			version:     "TLSv1.0",
			expected:    tls.VersionTLS10,
			expectError: false,
		},
		{
			name:        "TLS 1.3 with TLSv1.3",
			version:     "TLSv1.3",
			expected:    tls.VersionTLS13,
			expectError: false,
		},
		{
			name:        "invalid version",
			version:     "TLSv9.9",
			expected:    0,
			expectError: true,
		},
		{
			name:        "empty version",
			version:     "",
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTLSVersion(tt.version)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func Test_getTLSProfileSpec(t *testing.T) {
	tests := []struct {
		name        string
		profile     *configv1.TLSSecurityProfile
		expectError bool
	}{
		{
			name: "Intermediate profile type",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectError: false,
		},
		{
			name: "Custom profile with spec",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Custom profile without spec - should error",
			profile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: nil,
			},
			expectError: true,
		},
		{
			name: "Unknown profile type defaults to Intermediate",
			profile: &configv1.TLSSecurityProfile{
				Type: "",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getTLSProfileSpec(tt.profile)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func Test_buildTLSConfigFromProfile(t *testing.T) {
	tests := []struct {
		name        string
		profile     *configv1.TLSSecurityProfile
		expectError bool
		checkTLS13  bool
	}{
		{
			name: "Modern profile with TLS 1.3",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectError: false,
			checkTLS13:  true,
		},
		{
			name: "Custom profile with TLS 1.2",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers: []string{
							"ECDHE-ECDSA-AES128-GCM-SHA256",
							"ECDHE-RSA-AES128-GCM-SHA256",
						},
					},
				},
			},
			expectError: false,
			checkTLS13:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildTLSConfigFromProfile(tt.profile)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotZero(t, result.MinVersion)

				if tt.checkTLS13 {
					assert.Equal(t, uint16(tls.VersionTLS13), result.MinVersion)
					assert.Equal(t, uint16(tls.VersionTLS13), result.MaxVersion)
				} else {
					// For non-TLS 1.3 profiles, cipher suites should be configured
					assert.NotEmpty(t, result.CipherSuites)
				}
			}
		})
	}
}
