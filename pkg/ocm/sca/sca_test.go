package sca

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

const (
	entitlementPem    = "entitlement.pem"
	entitlementKeyPem = "entitlement-key.pem"
	secTestData       = "secret testing data"
	unexpectedDataErr = "unexpected data in %s secret"
	notFoundDataErr   = "can't find %s in the %s secret data"
)

var testingSCACertData = []CertData{
	{
		Cert: "testing-cert",
		Key:  "testing-key",
		ID:   "testing-id",
		Metadata: CertMetadata{
			Arch: "aarch64",
		},
		OrgID: "testing-org-id",
	},
	{
		Cert: "testing-cert",
		Key:  "testing-key",
		ID:   "testing-id",
		Metadata: CertMetadata{
			Arch: "x86_64",
		},
		OrgID: "testing-org-id",
	},
}

func Test_SCAController_SecretIsCreated(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()
	scaController := New(coreClient, nil, nil)

	testRes := &CertData{
		Key:  "secret key",
		Cert: "secret cert",
	}
	err := scaController.checkSecret(context.Background(), testRes, secretName)
	assert.NoError(t, err, "failed to check the secret")

	testSecret, err := coreClient.Secrets(targetNamespaceName).Get(context.Background(), secretName, metav1.GetOptions{})
	assert.NoError(t, err, "can't get secret")
	assert.Contains(t, testSecret.Data, entitlementKeyPem, notFoundDataErr, entitlementKeyPem, secretName)
	assert.Contains(t, testSecret.Data, entitlementPem, notFoundDataErr, entitlementPem, secretName)
	assert.Equal(t, "secret key", string(testSecret.Data[entitlementKeyPem]), unexpectedDataErr, secretName)
	assert.Equal(t, "secret cert", string(testSecret.Data[entitlementPem]), unexpectedDataErr, secretName)
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
	scaController := New(coreClient, nil, nil)
	testRes := &CertData{
		Key:  "new secret testing key",
		Cert: "new secret testing cert",
	}
	err = scaController.checkSecret(context.Background(), testRes, secretName)
	assert.NoError(t, err, "failed to check the secret")

	testSecret, err := coreClient.Secrets(targetNamespaceName).Get(context.Background(), secretName, metav1.GetOptions{})
	assert.NoError(t, err, "can't get secret")
	assert.Contains(t, testSecret.Data, entitlementKeyPem, notFoundDataErr, entitlementKeyPem, secretName)
	assert.Contains(t, testSecret.Data, entitlementPem, notFoundDataErr, entitlementPem, secretName)
	assert.Equal(t, "new secret testing key", string(testSecret.Data[entitlementKeyPem]), unexpectedDataErr, secretName)
	assert.Equal(t, "new secret testing cert", string(testSecret.Data[entitlementPem]), unexpectedDataErr, secretName)
}

func Test_SCAController_ProcessResponses(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()
	scaController := New(coreClient, nil, nil)

	testCases := []struct {
		name     string
		response Response
	}{
		{
			name: "single architecture",
			response: Response{
				Items: testingSCACertData[:1],
				Kind:  "EntitlementCertificatesList",
				Total: 1,
			},
		},
		{
			name: "multiple architectures",
			response: Response{
				Items: testingSCACertData,
				Kind:  "EntitlementCertificatesList",
				Total: 2,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := scaController.processResponses(context.Background(), tt.response)
			assert.NoError(t, err, "failed to process the response")

			for i, response := range tt.response.Items {
				testSecretName := secretName
				if tt.response.Total > 1 {
					// Multiple architectures should create a secret with the arch suffix
					testSecretName = fmt.Sprintf(secretArchName, archMapping[response.Metadata.Arch])
				}

				testSecret, err := coreClient.Secrets(targetNamespaceName).Get(
					context.Background(),
					testSecretName,
					metav1.GetOptions{},
				)

				assert.NoError(t, err, "can't get secret")
				assert.Contains(t, testSecret.Data, entitlementKeyPem, notFoundDataErr, entitlementKeyPem, secretName)
				assert.Contains(t, testSecret.Data, entitlementPem, notFoundDataErr, entitlementPem, secretName)
				assert.Equal(t, tt.response.Items[i].Key, string(testSecret.Data[entitlementKeyPem]), unexpectedDataErr, secretName)
				assert.Equal(t, tt.response.Items[i].Cert, string(testSecret.Data[entitlementPem]), unexpectedDataErr, secretName)
			}
		})
	}
}
