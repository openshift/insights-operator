package clusterconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_cleanCollectorSpecConfig(t *testing.T) {
	t.Run("valid spec.config with service field - only service is kept", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"config": map[string]interface{}{
						"service": map[string]interface{}{
							"telemetry": "test1",
							"receivers": "test2",
						}}}}}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)

		config, found, err := unstructured.NestedMap(item.Object, "spec", "config")
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Contains(t, config, "service")
		assert.NotNil(t, config["service"])

		service, _ := config["service"].(map[string]interface{})
		assert.Contains(t, service, "telemetry")   // field from stub
		assert.NotContains(t, config, "receivers") // the rest of the fields should be dropped
	})

	t.Run("missing spec.config - no error, item unchanged", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{},
			},
		}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
	})

	t.Run("unexpected spec.config value - returns a controlled error", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{"config": "test1"},
			},
		}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.Error(t, test)
		assert.ErrorContains(t, test, "accessor error")
	})

	t.Run("valid spec.config with NO service field - returns a cleaned field", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"config": map[string]interface{}{
						"receivers": map[string]interface{}{},
						"exporters": map[string]interface{}{},
					}}}}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
		config, found, err := unstructured.NestedMap(item.Object, "spec", "config")
		assert.NoError(t, err)
		assert.True(t, found)
		// receivers/exporters are always dropped
		assert.NotContains(t, config, "receivers")
		assert.NotContains(t, config, "exporters")
	})
}
