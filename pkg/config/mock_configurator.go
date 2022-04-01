package config

// MockConfigurator returns the config from conf field
type MockConfigurator struct {
	Conf *Controller
}

// NewMockConfigurator constructs a new MockConfigurator with default config values
func NewMockConfigurator(conf *Controller) *MockConfigurator {
	if conf == nil {
		conf = &Controller{}
	}
	if len(conf.Gather) == 0 {
		conf.Gather = []string{"ALL"}
	}
	return &MockConfigurator{
		Conf: conf,
	}
}

func (mc *MockConfigurator) Config() *Controller {
	return mc.Conf
}

func (mc *MockConfigurator) ConfigChanged() (<-chan struct{}, func()) { //nolint: gocritic
	return nil, func() {}
}
