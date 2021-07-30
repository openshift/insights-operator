package ocm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var (
	tlsSecretCrt = "tls.crt"
	tlsSecretKey = "tls.key"
	secTestData  = "secret testing data"
)

var testRes = &ScaResponse{
	Key:  "secret key",
	Cert: "secret cert",
}

func Test_OCMController_SecretIsCreated(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()
	ocmController := New(context.TODO(), coreClient, nil, nil)

	err := ocmController.checkSecret(testRes)
	assert.NoError(t, err, "failed to check the secret")

	testSecret, err := coreClient.Secrets(targetNamespaceName).Get(context.Background(), secretName, metav1.GetOptions{})
	assert.NoError(t, err, "can't get secret")
	assert.Contains(t, testSecret.Data, tlsSecretKey, "can't find %s in the %s secret data", tlsSecretKey, secretName)
	assert.Contains(t, testSecret.Data, tlsSecretCrt, "can't find %s in the %s secret data", tlsSecretCrt, secretName)
	assert.Equal(t, "secret key", string(testSecret.Data[tlsSecretKey]), "unexpected data in %s secret", secretName)
	assert.Equal(t, "secret cert", string(testSecret.Data[tlsSecretCrt]), "unexpected data in %s secret", secretName)
}

func Test_OCMController_SecretIsUpdated(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()

	existingSec := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte(secTestData),
			v1.TLSPrivateKeyKey: []byte(secTestData),
		},
	}
	_, err := coreClient.Secrets(targetNamespaceName).Create(context.Background(), existingSec, metav1.CreateOptions{})
	assert.NoError(t, err)
	ocmController := New(context.TODO(), coreClient, nil, nil)

	testRes.Cert = "new secret testing cert"
	testRes.Key = "new secret testing key"
	err = ocmController.checkSecret(testRes)
	assert.NoError(t, err, "failed to check the secret")

	testSecret, err := coreClient.Secrets(targetNamespaceName).Get(context.Background(), secretName, metav1.GetOptions{})
	assert.NoError(t, err, "can't get secret")
	assert.Contains(t, testSecret.Data, tlsSecretKey, "can't find %s in the %s secret data", tlsSecretKey, secretName)
	assert.Contains(t, testSecret.Data, tlsSecretCrt, "can't find %s in the %s secret data", tlsSecretCrt, secretName)
	assert.Equal(t, "new secret testing key", string(testSecret.Data[tlsSecretKey]), "unexpected data in %s secret", secretName)
	assert.Equal(t, "new secret testing cert", string(testSecret.Data[tlsSecretCrt]), "unexpected data in %s secret", secretName)
}
