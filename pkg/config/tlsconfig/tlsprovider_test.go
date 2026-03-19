package tlsconfig

import (
	"crypto/tls"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTLSConfigProvider(t *testing.T) {
	provider := NewTLSConfigProvider()

	assert.NotNil(t, provider)
	assert.NotNil(t, provider.observedConfig)
	assert.Nil(t, provider.tlsConfig)
}

func TestTLSConfigProvider_GetTLSConfig_Uninitialized(t *testing.T) {
	provider := NewTLSConfigProvider()

	config := provider.GetTLSConfig()

	assert.NotNil(t, config)
	assert.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
	assert.Nil(t, config.CipherSuites) // Default config has no cipher suites set
}

func TestTLSConfigProvider_UpdateObservedConfig_Valid(t *testing.T) {
	provider := NewTLSConfigProvider()

	observedConfig := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS12",
			"cipherSuites": []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			},
		},
	}

	err := provider.UpdateObservedConfig(observedConfig)

	assert.NoError(t, err)
	assert.NotNil(t, provider.tlsConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), provider.tlsConfig.MinVersion)
	assert.Len(t, provider.tlsConfig.CipherSuites, 2)
}

func TestTLSConfigProvider_UpdateObservedConfig_Invalid(t *testing.T) {
	tests := []struct {
		name           string
		observedConfig map[string]interface{}
	}{
		{
			name: "missing servingInfo",
			observedConfig: map[string]interface{}{
				"other": "value",
			},
		},
		{
			name: "invalid minTLSVersion",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "InvalidVersion",
					"cipherSuites":  []string{},
				},
			},
		},
		{
			name: "no valid cipher suites",
			observedConfig: map[string]interface{}{
				"servingInfo": map[string]interface{}{
					"minTLSVersion": "VersionTLS12",
					"cipherSuites": []string{
						"INVALID_CIPHER",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewTLSConfigProvider()

			err := provider.UpdateObservedConfig(tt.observedConfig)

			assert.Error(t, err)
			// Provider should still be in a safe state even after error
			config := provider.GetTLSConfig()
			assert.NotNil(t, config)
		})
	}
}

func TestTLSConfigProvider_GetTLSConfig_AfterUpdate(t *testing.T) {
	provider := NewTLSConfigProvider()

	observedConfig := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS13",
			"cipherSuites":  []string{}, // TLS 1.3 has fixed ciphers
		},
	}

	err := provider.UpdateObservedConfig(observedConfig)
	assert.NoError(t, err)

	config := provider.GetTLSConfig()

	assert.NotNil(t, config)
	assert.Equal(t, uint16(tls.VersionTLS13), config.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), config.MaxVersion)
}

func TestTLSConfigProvider_GetTLSConfig_ReturnsClone(t *testing.T) {
	provider := NewTLSConfigProvider()

	observedConfig := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS12",
			"cipherSuites": []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}

	err := provider.UpdateObservedConfig(observedConfig)
	assert.NoError(t, err)

	// Get two configs
	config1 := provider.GetTLSConfig()
	config2 := provider.GetTLSConfig()

	// They should be equal but not the same object
	assert.Equal(t, config1.MinVersion, config2.MinVersion)
	assert.NotSame(t, config1, config2)

	// Modifying one should not affect the other
	config1.ServerName = "test.example.com"
	assert.Empty(t, config2.ServerName)
}

func TestTLSConfigProvider_ThreadSafety(t *testing.T) {
	provider := NewTLSConfigProvider()

	observedConfig := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS12",
			"cipherSuites": []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}

	var wg sync.WaitGroup
	numReaders := 10
	numWriters := 5

	// Start multiple readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				config := provider.GetTLSConfig()
				assert.NotNil(t, config)
			}
		}()
	}

	// Start multiple writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = provider.UpdateObservedConfig(observedConfig)
			}
		}()
	}

	wg.Wait()

	// Final config should be valid
	finalConfig := provider.GetTLSConfig()
	assert.NotNil(t, finalConfig)
}

func TestTLSConfigProvider_MultipleUpdates(t *testing.T) {
	provider := NewTLSConfigProvider()

	// First update - TLS 1.2
	config1 := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS12",
			"cipherSuites": []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}
	err := provider.UpdateObservedConfig(config1)
	assert.NoError(t, err)

	tlsConfig := provider.GetTLSConfig()
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)

	// Second update - TLS 1.3
	config2 := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS13",
			"cipherSuites":  []string{},
		},
	}
	err = provider.UpdateObservedConfig(config2)
	assert.NoError(t, err)

	tlsConfig = provider.GetTLSConfig()
	assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MaxVersion)
}
