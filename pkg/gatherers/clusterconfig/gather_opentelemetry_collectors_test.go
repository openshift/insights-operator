package clusterconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const testLastAppliedConfiguration = `{"spec":{"config":{"exporters":{"otlphttp":{"endpoint":"https://otlp.vendor.com:4318"}}}}}`

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

	t.Run("missing spec.config - last-applied-configuration annotation is still stripped", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/last-applied-configuration": testLastAppliedConfiguration,
					},
				},
				"spec": map[string]interface{}{},
			},
		}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
		assert.NotContains(t, item.GetAnnotations(), "kubectl.kubernetes.io/last-applied-configuration")
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
		// receivers are always dropped, and an empty exporters map yields no otlp exporters
		assert.NotContains(t, config, "receivers")
		assert.NotContains(t, config, "exporters")
	})

	t.Run("otlp exporters endpoints are kept and anonymized, non-otlp exporters are dropped", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"config": map[string]interface{}{
						"exporters": map[string]interface{}{
							"otlp": map[string]interface{}{
								"endpoint": "tempo-collector.tracing.svc:4317",
							},
							"otlpgrpc/myexporter": map[string]interface{}{
								"endpoint": "https://otlp.vendor.com:4317",
							},
							"otlphttp": map[string]interface{}{
								"endpoint": "https://otlp.vendor.com:4318",
							},
							"otlp_grpc": map[string]interface{}{
								"endpoint": "0.0.0.0:4317",
							},
							"debug": map[string]interface{}{},
						},
					}}}}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
		config, found, err := unstructured.NestedMap(item.Object, "spec", "config")
		assert.NoError(t, err)
		assert.True(t, found)

		exporters, ok := config["exporters"].(map[string]interface{})
		assert.True(t, ok)
		assert.NotContains(t, exporters, "debug")
		assert.Len(t, exporters, 4)

		otlp, ok := exporters["otlp"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "xxxxx-xxxxxxxxx.xxxxxxx.xxx:xxxx", otlp["endpoint"])

		otlpGrpcNamed, ok := exporters["otlpgrpc/myexporter"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "xxxxx://xxxx.xxxxxx.xxx:xxxx", otlpGrpcNamed["endpoint"])
	})

	t.Run("otlp exporter without endpoint field is dropped", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"config": map[string]interface{}{
						"exporters": map[string]interface{}{
							"otlp": map[string]interface{}{
								"tls": map[string]interface{}{"insecure": true},
							},
						},
					}}}}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
		config, found, err := unstructured.NestedMap(item.Object, "spec", "config")
		assert.NoError(t, err)
		assert.True(t, found)
		assert.NotContains(t, config, "exporters")
	})

	t.Run("last-applied-configuration annotation is stripped", func(t *testing.T) {
		// given
		item := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"kubectl.kubernetes.io/last-applied-configuration": testLastAppliedConfiguration,
						"some-other-annotation":                            "keep-me",
					},
				},
				"spec": map[string]interface{}{
					"config": map[string]interface{}{
						"service": map[string]interface{}{},
					}}}}

		// when
		test := cleanCollectorSpecConfig(item)

		// assert
		assert.NoError(t, test)
		annotations := item.GetAnnotations()
		assert.NotContains(t, annotations, "kubectl.kubernetes.io/last-applied-configuration")
		assert.Equal(t, "keep-me", annotations["some-other-annotation"])
	})
}

func Test_otlpExportersEndpoints(t *testing.T) {
	t.Run("non-map input returns nil", func(t *testing.T) {
		assert.Nil(t, otlpExportersEndpoints("not-a-map"))
		assert.Nil(t, otlpExportersEndpoints(nil))
	})

	t.Run("filters out non-otlp exporter names", func(t *testing.T) {
		exporters := map[string]interface{}{
			"otlploggy":  map[string]interface{}{"endpoint": "example.com:1234"},
			"prometheus": map[string]interface{}{"endpoint": "example.com:1234"},
		}

		result := otlpExportersEndpoints(exporters)

		assert.Empty(t, result)
	})
}
