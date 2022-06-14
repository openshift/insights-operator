package insights

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

// MustRegisterMetrics registers provided registrables in the Insights metrics registry.
// This function should be called from init() functions only, because
// it uses the MustRegister method, and therefore panics in case of an error.
func MustRegisterMetrics(registrables ...metrics.Registerable) {
	for _, r := range registrables {
		err := legacyregistry.Register(r)
		if err != nil {
			klog.Errorf("Failed to register metric %s: %v", r.FQName(), err)
		}
	}
}
