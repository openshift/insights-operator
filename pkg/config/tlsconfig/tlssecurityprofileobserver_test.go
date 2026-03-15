package tlsconfig

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// mockAPIServerLister implements configlistersv1.APIServerLister for testing
type mockAPIServerLister struct {
	apiServer *configv1.APIServer
	err       error
}

func (m *mockAPIServerLister) List(_ labels.Selector) ([]*configv1.APIServer, error) {
	if m.apiServer == nil {
		return []*configv1.APIServer{}, m.err
	}
	return []*configv1.APIServer{m.apiServer}, m.err
}

func (m *mockAPIServerLister) Get(name string) (*configv1.APIServer, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.apiServer == nil {
		return nil, errors.NewNotFound(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, name)
	}
	return m.apiServer, nil
}

// createTestListers creates a TLSProfileListers with a mock APIServerLister for testing
func createTestListers(apiServer *configv1.APIServer, err error) *TLSProfileListers {
	mockLister := &mockAPIServerLister{
		apiServer: apiServer,
		err:       err,
	}

	synced := func() bool { return true }

	return NewTLSProfileListers(mockLister, nil, []cache.InformerSynced{synced})
}

// mockEventRecorder implements events.Recorder for testing
type mockEventRecorder struct {
	events []string
}

func (m *mockEventRecorder) Event(reason, message string) {
	m.events = append(m.events, reason+": "+message)
}

func (m *mockEventRecorder) Eventf(reason, _ string, _ ...interface{}) {
	m.events = append(m.events, reason)
}

func (m *mockEventRecorder) Warning(reason, message string) {
	m.events = append(m.events, "WARNING: "+reason+": "+message)
}

func (m *mockEventRecorder) Warningf(reason, _ string, _ ...interface{}) {
	m.events = append(m.events, "WARNING: "+reason)
}

func (m *mockEventRecorder) ForComponent(_ string) events.Recorder {
	return m
}

func (m *mockEventRecorder) WithComponentSuffix(_ string) events.Recorder {
	return m
}

func (m *mockEventRecorder) WithContext(_ context.Context) events.Recorder {
	return m
}

func (m *mockEventRecorder) ComponentName() string {
	return "test"
}

func (m *mockEventRecorder) Shutdown() {}

func TestObserveTLSSecurityProfile_IntermediateProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
		},
	}

	listers := createTestListers(apiServer, nil)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, map[string]interface{}{})

	assert.Empty(t, errs)
	assert.NotNil(t, observedConfig)

	// Check servingInfo structure
	servingInfo, ok := observedConfig["servingInfo"].(map[string]interface{})
	assert.True(t, ok)
	assert.NotNil(t, servingInfo)

	// Intermediate profile should have TLS 1.2
	minVersion, ok := servingInfo["minTLSVersion"].(string)
	assert.True(t, ok)
	assert.Equal(t, "VersionTLS12", minVersion)

	// Should have cipher suites
	ciphers, ok := servingInfo["cipherSuites"].([]string)
	assert.True(t, ok)
	assert.NotEmpty(t, ciphers)
}

func TestObserveTLSSecurityProfile_ModernProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}

	listers := createTestListers(apiServer, nil)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, map[string]interface{}{})

	assert.Empty(t, errs)
	assert.NotNil(t, observedConfig)

	servingInfo, ok := observedConfig["servingInfo"].(map[string]interface{})
	assert.True(t, ok)

	// Modern profile should have TLS 1.3
	minVersion, ok := servingInfo["minTLSVersion"].(string)
	assert.True(t, ok)
	assert.Equal(t, "VersionTLS13", minVersion)
}

func TestObserveTLSSecurityProfile_OldProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
		},
	}

	listers := createTestListers(apiServer, nil)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, map[string]interface{}{})

	assert.Empty(t, errs)
	assert.NotNil(t, observedConfig)

	servingInfo, ok := observedConfig["servingInfo"].(map[string]interface{})
	assert.True(t, ok)

	// Old profile should have TLS 1.0
	minVersion, ok := servingInfo["minTLSVersion"].(string)
	assert.True(t, ok)
	assert.Equal(t, "VersionTLS10", minVersion)
}

func TestObserveTLSSecurityProfile_CustomProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers: []string{
							"ECDHE-RSA-AES128-GCM-SHA256",
						},
					},
				},
			},
		},
	}

	listers := createTestListers(apiServer, nil)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, map[string]interface{}{})

	assert.Empty(t, errs)
	assert.NotNil(t, observedConfig)

	servingInfo, ok := observedConfig["servingInfo"].(map[string]interface{})
	assert.True(t, ok)

	minVersion, ok := servingInfo["minTLSVersion"].(string)
	assert.True(t, ok)
	assert.Equal(t, "VersionTLS12", minVersion)

	ciphers, ok := servingInfo["cipherSuites"].([]string)
	assert.True(t, ok)
	assert.Len(t, ciphers, 1)
}

func TestObserveTLSSecurityProfile_NoProfile(t *testing.T) {
	// No TLS profile specified - should use default Intermediate
	apiServer := &configv1.APIServer{
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: nil,
		},
	}

	listers := createTestListers(apiServer, nil)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, map[string]interface{}{})

	assert.Empty(t, errs)
	assert.NotNil(t, observedConfig)

	servingInfo, ok := observedConfig["servingInfo"].(map[string]interface{})
	assert.True(t, ok)

	// Should default to Intermediate (TLS 1.2)
	minVersion, ok := servingInfo["minTLSVersion"].(string)
	assert.True(t, ok)
	assert.Equal(t, "VersionTLS12", minVersion)
}

func TestObserveTLSSecurityProfile_APIServerNotFound(t *testing.T) {
	// APIServer resource doesn't exist
	listers := createTestListers(nil, nil)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, map[string]interface{}{})

	// Should not error, should use default Intermediate
	assert.Empty(t, errs)
	assert.NotNil(t, observedConfig)

	servingInfo, ok := observedConfig["servingInfo"].(map[string]interface{})
	assert.True(t, ok)

	minVersion, ok := servingInfo["minTLSVersion"].(string)
	assert.True(t, ok)
	assert.Equal(t, "VersionTLS12", minVersion)
}

func TestObserveTLSSecurityProfile_GetError(t *testing.T) {
	existingConfig := map[string]interface{}{
		"servingInfo": map[string]interface{}{
			"minTLSVersion": "VersionTLS10",
		},
	}

	listers := createTestListers(nil, assert.AnError)

	recorder := &mockEventRecorder{}

	observedConfig, errs := ObserveTLSSecurityProfile(listers, recorder, existingConfig)

	// Should preserve existing config on error
	assert.NotEmpty(t, errs)
	assert.Equal(t, existingConfig, observedConfig)
}

func TestGetTLSProfileSpec(t *testing.T) {
	tests := []struct {
		name            string
		profile         *configv1.TLSSecurityProfile
		expectError     bool
		expectedMinVer  configv1.TLSProtocolVersion
		expectedCiphers int
	}{
		{
			name:           "nil profile defaults to Intermediate",
			profile:        nil,
			expectError:    false,
			expectedMinVer: configv1.VersionTLS12,
		},
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectError:    false,
			expectedMinVer: configv1.VersionTLS12,
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectError:    false,
			expectedMinVer: configv1.VersionTLS13,
		},
		{
			name: "Old profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			expectError:    false,
			expectedMinVer: configv1.VersionTLS10,
		},
		{
			name: "Custom profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS13,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
			expectError:     false,
			expectedMinVer:  configv1.VersionTLS13,
			expectedCiphers: 1,
		},
		{
			name: "Custom profile with nil Custom field",
			profile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: nil,
			},
			expectError: true,
		},
		{
			name: "Unknown profile type defaults to Intermediate",
			profile: &configv1.TLSSecurityProfile{
				Type: "UnknownType",
			},
			expectError:    false,
			expectedMinVer: configv1.VersionTLS12, // Falls back to Intermediate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := getTLSProfileSpec(tt.profile)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, spec)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, spec)
				assert.Equal(t, tt.expectedMinVer, spec.MinTLSVersion)
				if tt.expectedCiphers > 0 {
					assert.Len(t, spec.Ciphers, tt.expectedCiphers)
				}
			}
		})
	}
}

func TestGetProfileTypeName(t *testing.T) {
	tests := []struct {
		name     string
		profile  *configv1.TLSSecurityProfile
		expected string
	}{
		{
			name:     "nil profile",
			profile:  nil,
			expected: "Intermediate",
		},
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expected: "Intermediate",
		},
		{
			name: "Modern profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expected: "Modern",
		},
		{
			name: "Custom profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
			},
			expected: "Custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getProfileTypeName(tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetObservedField(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		value         interface{}
		expectError   bool
		validateValue func(t *testing.T, config map[string]interface{})
	}{
		{
			name:        "simple two-level path",
			path:        "servingInfo.minTLSVersion",
			value:       "VersionTLS12",
			expectError: false,
			validateValue: func(t *testing.T, config map[string]interface{}) {
				servingInfo, ok := config["servingInfo"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "VersionTLS12", servingInfo["minTLSVersion"])
			},
		},
		{
			name:        "set cipher suites",
			path:        "servingInfo.cipherSuites",
			value:       []string{"cipher1", "cipher2"},
			expectError: false,
			validateValue: func(t *testing.T, config map[string]interface{}) {
				servingInfo, ok := config["servingInfo"].(map[string]interface{})
				assert.True(t, ok)
				ciphers, ok := servingInfo["cipherSuites"].([]string)
				assert.True(t, ok)
				assert.Len(t, ciphers, 2)
			},
		},
		{
			name:        "empty path",
			path:        "",
			value:       "test",
			expectError: true,
		},
		{
			name:        "single level path",
			path:        "single",
			value:       "value",
			expectError: false,
			validateValue: func(t *testing.T, config map[string]interface{}) {
				assert.Equal(t, "value", config["single"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := make(map[string]interface{})
			err := setObservedField(config, tt.path, tt.value)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateValue != nil {
					tt.validateValue(t, config)
				}
			}
		})
	}
}

func TestNewTLSProfileListers(t *testing.T) {
	mockLister := &mockAPIServerLister{}
	synced := func() bool { return true }

	listers := NewTLSProfileListers(mockLister, nil, []cache.InformerSynced{synced})

	assert.NotNil(t, listers)
	assert.Equal(t, mockLister, listers.APIServerLister())
	assert.Nil(t, listers.ResourceSyncer())
	assert.Len(t, listers.PreRunHasSynced(), 1)
}
