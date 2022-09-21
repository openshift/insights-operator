package config

import (
	"github.com/openshift/api/config/v1alpha1"
	"github.com/openshift/insights-operator/pkg/utils"
	"github.com/openshift/library-go/pkg/controller/factory"
)

// MockSecretConfigurator returns the config from conf field
type MockSecretConfigurator struct {
	Conf *Controller
}

// NewMockConfigurator constructs a new MockConfigurator with default config values
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
		if utils.StringInSlice("all", mc.config.DisabledGatherers) ||
			utils.StringInSlice("ALL", mc.config.DisabledGatherers) {
			return true
		}
	}
	return false
}
