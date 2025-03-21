package config

import (
	"context"
	"slices"

	"github.com/openshift/api/config/v1alpha1"
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
	config *v1alpha1.GatherConfig
}

// NewMockAPIConfigurator constructs a new NewMockAPIConfigurator with provided GatherConfig values
func NewMockAPIConfigurator(gatherConfig *v1alpha1.GatherConfig) *MockAPIConfigurator {
	mockAPIConf := &MockAPIConfigurator{
		config: gatherConfig,
	}
	return mockAPIConf
}

func (mc *MockAPIConfigurator) GatherConfig() *v1alpha1.GatherConfig {
	return mc.config
}

func (mc *MockAPIConfigurator) GatherDisabled() bool {
	if mc.config != nil {
		if slices.Contains(mc.config.DisabledGatherers, v1alpha1.DisabledGatherer("all")) ||
			slices.Contains(mc.config.DisabledGatherers, v1alpha1.DisabledGatherer("ALL")) {
			return true
		}
	}
	return false
}

func (mc *MockAPIConfigurator) GatherDataPolicy() *v1alpha1.DataPolicy {
	if mc.config != nil {
		return &mc.config.DataPolicy
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
