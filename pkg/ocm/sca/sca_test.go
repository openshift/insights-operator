package sca

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var (
	entitlementPem    = "entitlement.pem"
	entitlementKeyPem = "entitlement-key.pem"
	secTestData       = "secret testing data"
)

func Test_SCAController_SecretIsCreated(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()
	scaController := New(context.TODO(), coreClient, nil, nil)

	testRes := &Response{
		Key:  "secret key",
		Cert: "secret cert",
	}
	err := scaController.checkSecret(testRes)
	assert.NoError(t, err, "failed to check the secret")

	testSecret, err := coreClient.Secrets(targetNamespaceName).Get(context.Background(), secretName, metav1.GetOptions{})
	assert.NoError(t, err, "can't get secret")
	assert.Contains(t, testSecret.Data, entitlementKeyPem, "can't find %s in the %s secret data", entitlementKeyPem, secretName)
	assert.Contains(t, testSecret.Data, entitlementPem, "can't find %s in the %s secret data", entitlementPem, secretName)
	assert.Equal(t, "secret key", string(testSecret.Data[entitlementKeyPem]), "unexpected data in %s secret", secretName)
	assert.Equal(t, "secret cert", string(testSecret.Data[entitlementPem]), "unexpected data in %s secret", secretName)
}

func Test_SCAController_SecretIsUpdated(t *testing.T) {
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
	scaController := New(context.TODO(), coreClient, nil, nil)
	testRes := &Response{
		Key:  "new secret testing key",
		Cert: "new secret testing cert",
	}
	err = scaController.checkSecret(testRes)
	assert.NoError(t, err, "failed to check the secret")

	testSecret, err := coreClient.Secrets(targetNamespaceName).Get(context.Background(), secretName, metav1.GetOptions{})
	assert.NoError(t, err, "can't get secret")
	assert.Contains(t, testSecret.Data, entitlementKeyPem, "can't find %s in the %s secret data", entitlementKeyPem, secretName)
	assert.Contains(t, testSecret.Data, entitlementPem, "can't find %s in the %s secret data", entitlementPem, secretName)
	assert.Equal(t, "new secret testing key", string(testSecret.Data[entitlementKeyPem]), "unexpected data in %s secret", secretName)
	assert.Equal(t, "new secret testing cert", string(testSecret.Data[entitlementPem]), "unexpected data in %s secret", secretName)
}
