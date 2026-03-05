package clusterconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_parseCollectorSpecConfig(t *testing.T) {
	cfgStub := `
service:
  telemetry:
    logs:
      level: info
receivers:
  hostmetrics:
    scrapers:
      cpu: {}
`
	cfgStubNoService := `
receivers:
  hostmetrics: {}
exporters:
  debug: {}
`

	t.Run("valid config with service field - only service is kept", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{"config": cfgStub},
			},
		}

		// when
		test := parseCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)

		config, found, test := unstructured.NestedMap(item.Object, "spec", "config")
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
		test := parseCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
	})

	t.Run("invalid YAML in spec.config - error", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{"config": "invalid: yaml: [unclosed"},
			},
		}

		// when
		test := parseCollectorSpecConfig(item)

		// assert
		assert.Error(t, test)
		assert.ErrorContains(t, test, "error converting YAML")
	})

	t.Run("valid YAML with no service key - config becomes service only with null", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{"config": cfgStubNoService},
			},
		}

		// when
		test := parseCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
		config, found, test := unstructured.NestedMap(item.Object, "spec", "config")
		assert.True(t, found)
		assert.Contains(t, config, "service")
		// receivers/exporters are always dropped
		assert.NotContains(t, config, "receivers")
		assert.NotContains(t, config, "exporters")
	})
}
