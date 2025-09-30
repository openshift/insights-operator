package config

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/controller/factory"
)

// MockSecretConfigurator returns the config from conf field
type MockSecretConfigurator struct {
	Conf *Controller
}

// NewMockSecretConfigurator constructs a new MockConfigurator with default config values
func NewMockSecretConfigurator(conf *Controller) *MockSecretConfigurator {
	if conf == nil {
		conf = &Controller{}
	}
	return &MockSecretConfigurator{
		Conf: conf,
	}
}

func (mc *MockSecretConfigurator) Config() *Controller {
	return mc.Conf
}

func (mc *MockSecretConfigurator) ConfigChanged() (<-chan struct{}, func()) { //nolint: gocritic
	return nil, func() {}
}

type MockAPIConfigurator struct {
	factory.Controller
	config *configv1.GatherConfig
}

// NewMockAPIConfigurator constructs a new NewMockAPIConfigurator with provided GatherConfig values
func NewMockAPIConfigurator(gatherConfig *configv1.GatherConfig) *MockAPIConfigurator {
	mockAPIConf := &MockAPIConfigurator{
		config: gatherConfig,
	}
	return mockAPIConf
}

func (mc *MockAPIConfigurator) GatherConfig() *configv1.GatherConfig {
	return mc.config
}

func (mc *MockAPIConfigurator) GatherDisabled() bool {
	return mc.config.Gatherers.Mode == configv1.GatheringModeNone
}

func (mc *MockAPIConfigurator) GatherDataPolicy() []configv1.DataPolicyOption {
	if mc.config != nil {
		return mc.config.DataPolicy
	}
	return nil
}

type MockConfigMapConfigurator struct {
	factory.Controller
	insightsConfig *InsightsConfiguration
}

func NewMockConfigMapConfigurator(config *InsightsConfiguration) *MockConfigMapConfigurator {
	return &MockConfigMapConfigurator{
		insightsConfig: config,
	}
}

func (m *MockConfigMapConfigurator) Config() *InsightsConfiguration {
	return m.insightsConfig
}

func (m *MockConfigMapConfigurator) ConfigChanged() (configCh <-chan struct{}, closeFn func()) {
	// noop
	return nil, func() {}
}

func (m *MockConfigMapConfigurator) Listen(context.Context) {
}
