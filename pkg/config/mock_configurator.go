package config

import (
	"context"

	"github.com/openshift/api/config/v1alpha2"
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
	config *v1alpha2.GatherConfig
}

// NewMockAPIConfigurator constructs a new NewMockAPIConfigurator with provided GatherConfig values
func NewMockAPIConfigurator(gatherConfig *v1alpha2.GatherConfig) *MockAPIConfigurator {
	mockAPIConf := &MockAPIConfigurator{
		config: gatherConfig,
	}
	return mockAPIConf
}

func (mc *MockAPIConfigurator) GatherConfig() *v1alpha2.GatherConfig {
	return mc.config
}

func (mc *MockAPIConfigurator) GatherDisabled() bool {
	return mc.config.Gatherers.Mode == v1alpha2.GatheringModeNone
}

func (mc *MockAPIConfigurator) GatherDataPolicy() []v1alpha2.DataPolicyOption {
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
