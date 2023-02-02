package anonymize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_EnvVar_Obfuscation(t *testing.T) {
	// Given
	mock := []corev1.Container{
		{
			Env: []corev1.EnvVar{
				{Name: "NO_TARGET", Value: "original_value"},
				{Name: "HTTP_PROXY", Value: "original_value"},
				{Name: "HTTPS_PROXY", Value: "original_value"},
			},
		},
	}
	envOriginalValue := "original_value"

	// When
	SensitiveEnvVars(mock)

	// Assert
	t.Run("Non target env vars keep their original value", func(t *testing.T) {
		test := mock[0].Env[0]
		assert.Equal(t, envOriginalValue, test.Value)
	})
	t.Run("HTTP_PROXY is updated with obfuscated value", func(t *testing.T) {
		test := mock[0].Env[1]
		assert.NotEqual(t, envOriginalValue, test.Value)
	})
	t.Run("HTTPS_PROXY is updated with obfuscated value", func(t *testing.T) {
		test := mock[0].Env[2]
		assert.NotEqual(t, envOriginalValue, test.Value)
	})
}
