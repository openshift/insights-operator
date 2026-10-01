package configobserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_tokenFromSecret_invalidJSON_returnsError(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte("{not-valid-json"),
		},
	}

	token, err := tokenFromSecret(secret)

	assert.Empty(t, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to unmarshal cluster pull-secret")
}
