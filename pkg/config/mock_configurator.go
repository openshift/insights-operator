package config

// MockConfigurator returns the config from conf field
type MockConfigurator struct {
	Conf *Controller
}

func (mc *MockConfigurator) Config() *Controller {
	return mc.Conf
}

// nolint: gocritic
func (mc *MockConfigurator) ConfigChanged() (<-chan struct{}, func()) {
	return nil, nil
}
