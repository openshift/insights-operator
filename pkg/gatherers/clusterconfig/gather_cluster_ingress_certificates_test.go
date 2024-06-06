package clusterconfig

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorfake "github.com/openshift/client-go/operator/clientset/versioned/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
)

func Test_gatherClusterIngressCertificates(t *testing.T) {
	// Mocks
	mockBytes, mockX509 := getCertMock()

	tests := []struct {
		name         string
		ingressDef   []operatorv1.IngressController
		secretDef    []corev1.Secret
		wantRecords  []record.Record
		wantErrCount int
	}{
		{
			name: "Custom Ingress controller with a cluster certificate is added to the collection",
			ingressDef: []operatorv1.IngressController{{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ingress-controller", Namespace: "openshift-ingress-operator"},
				Spec: operatorv1.IngressControllerSpec{DefaultCertificate: &corev1.LocalObjectReference{
					Name: "router-ca"},
				}}},
			secretDef: []corev1.Secret{{
				ObjectMeta: metav1.ObjectMeta{Name: "router-ca", Namespace: "openshift-ingress-operator"},
				Data:       map[string][]byte{"tls.crt": mockBytes},
			}, {
				ObjectMeta: metav1.ObjectMeta{Name: "router-certs-default", Namespace: "openshift-ingress"},
				Data:       map[string][]byte{"tls.crt": mockBytes},
			}},
			wantRecords: []record.Record{{
				Name: "aggregated/ingress_controllers_certs",
				Item: record.JSONMarshaller{
					Object: []*CertificateInfo{{
						Name:      "router-ca",
						Namespace: "openshift-ingress-operator",
						NotBefore: mockX509.NotBefore, NotAfter: mockX509.NotAfter,
						Controllers: []ControllerInfo{
							{Name: "test-ingress-controller", Namespace: "openshift-ingress-operator"},
						},
					}, {
						Name:      "router-certs-default",
						Namespace: "openshift-ingress",
						NotBefore: mockX509.NotBefore, NotAfter: mockX509.NotAfter,
						Controllers: []ControllerInfo{},
					}},
				},
			}},
		}, {
			name: "Custom Ingress Controller with custom certificate adds a new entry to the collection",
			ingressDef: []operatorv1.IngressController{{
				ObjectMeta: metav1.ObjectMeta{Name: "test-custom-ingress", Namespace: "openshift-ingress-operator"},
				Spec: operatorv1.IngressControllerSpec{DefaultCertificate: &corev1.LocalObjectReference{
					Name: "test-custom-secret"},
				}}},
			secretDef: []corev1.Secret{{
				ObjectMeta: metav1.ObjectMeta{Name: "router-ca", Namespace: "openshift-ingress-operator"},
				Data:       map[string][]byte{"tls.crt": mockBytes},
			}, {
				ObjectMeta: metav1.ObjectMeta{Name: "router-certs-default", Namespace: "openshift-ingress"},
				Data:       map[string][]byte{"tls.crt": mockBytes},
			}, {
				ObjectMeta: metav1.ObjectMeta{Name: "test-custom-secret", Namespace: "openshift-ingress-operator"},
				Data:       map[string][]byte{"tls.crt": mockBytes},
			}},
			wantRecords: []record.Record{{
				Name: "aggregated/ingress_controllers_certs",
				Item: record.JSONMarshaller{
					Object: []*CertificateInfo{{
						Name:      "router-ca",
						Namespace: "openshift-ingress-operator",
						NotBefore: mockX509.NotBefore, NotAfter: mockX509.NotAfter,
						Controllers: []ControllerInfo{},
					}, {
						Name:      "router-certs-default",
						Namespace: "openshift-ingress",
						NotBefore: mockX509.NotBefore, NotAfter: mockX509.NotAfter,
						Controllers: []ControllerInfo{},
					}, {
						Name:      "test-custom-secret",
						Namespace: "openshift-ingress-operator",
						NotBefore: mockX509.NotBefore, NotAfter: mockX509.NotAfter,
						Controllers: []ControllerInfo{
							{Name: "test-custom-ingress", Namespace: "openshift-ingress-operator"},
						},
					}},
				},
			}},
		}, {
			name: "Cluster default certificates that fail to validate return an error per cert",
			secretDef: []corev1.Secret{{
				ObjectMeta: metav1.ObjectMeta{Name: "router-ca", Namespace: "openshift-ingress-operator"},
			}, {
				ObjectMeta: metav1.ObjectMeta{Name: "router-certs-default", Namespace: "openshift-ingress"},
			}},
			wantErrCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			operatorClient := operatorfake.NewSimpleClientset()
			for _, ic := range tt.ingressDef {
				assert.NoError(t,
					operatorClient.Tracker().Add(ic.DeepCopy()))
			}
			coreClient := corefake.NewSimpleClientset()
			for _, sec := range tt.secretDef {
				assert.NoError(t,
					coreClient.Tracker().Add(&sec))
			}

			// When
			records, errs := gatherClusterIngressCertificates(context.TODO(), coreClient.CoreV1(), operatorClient)

			// Assert
			assert.EqualValues(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrCount)
		})
	}
}

func Test_getCertificateInfoFromSecret(t *testing.T) {
	// Mocks
	mockBytes, mockX509 := getCertMock()

	testCases := []struct {
		name            string
		secret          *corev1.Secret
		expected        *CertificateInfo
		expectErr       bool
		secretName      string
		secretNamespace string
	}{
		{
			name:            "a Secret containing a certificate returns a CertificateInfo struct",
			secretName:      "test",
			secretNamespace: "test-namespace",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-namespace"},
				Data:       map[string][]byte{"tls.crt": mockBytes},
			},
			expected: &CertificateInfo{
				Name: "test", Namespace: "test-namespace",
				NotBefore: mockX509.NotBefore, NotAfter: mockX509.NotAfter,
				Controllers: []ControllerInfo{},
			},
		}, {
			name:            "a Secret without a certificate returns an error",
			secretName:      "test",
			secretNamespace: "test-namespace",
			secret:          &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-namespace"}},
			expectErr:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			client := corefake.NewSimpleClientset(tc.secret)

			// When
			test, err := getCertificateInfoFromSecret(
				context.Background(), client.CoreV1(), tc.secretNamespace, tc.secretName)

			// Assert
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, tc.expected, test)
		})
	}
}

func getCertMock() ([]byte, *x509.Certificate) {
	mockCertbytes := []byte(`-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUfTstqHMAhGLL+j3n6pmwLw8vt84wDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yNDAzMDYxMjU5MTFaFw0yNTAz
MDYxMjU5MTFaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDoqkPZMeMi5qjkG384ZwpAc3QScOGYBWOEDFAioq5C
YhtDGBSMq2VwS0r8RvEEhbebvXuH5PLcIuEVO/MZRQD9gSacCfLlMLRKZYpv168m
KUYyhx1bXKmUlbQxCnpAPZ7nf14A3Pb0TzLfsKjoUdUOv/1eorA6+oU78StWx/Nt
W94ad9n3o+cjiMPu/RS3g9b+x07bG5mFYuzpWk/Svb5Lb42g8AtonzqFJBbhlStU
A+9UyzmyXMeTlbI9fFmku7mb5Uq0SZ8jhpH+fyCoQOxefTfVrvjrkkdavDn43hjz
5hCwGJ3mV96MU9hh398oBguOHaJ6V3/UHtW1spsFY83RAgMBAAGjUzBRMB0GA1Ud
DgQWBBR/zvuHjFadvifzAGBHegOxmRXnCzAfBgNVHSMEGDAWgBR/zvuHjFadvifz
AGBHegOxmRXnCzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBs
E2U+jQzJuEt9e6UEnS1T0cb2NxaGb7CYsSX0TjZK1VgloAKbnxaCLjRruTOOwfm6
s5CFzFjJoIhUASzoA295Np2AR0UEYr5fendIjKCztCMlpj0fp92jFL6/RZWNGM1A
qECHYtZckeqJjg9vUdfHtiBRoyEHJUJ/tsDDlslwzocdJUqKL8V6KerZsh5SIAkS
rJ8EgVyDvwQaLPQMttjk62croI1Wi3FLmkvvtTbNcMgTnVhFfGjyHOiGnQeBfqax
5P0VBuAUCihegskKEUCJB8HFPkC4hqbrEk0+psQ2Gm8kjoll/SpltFLS77Xjhrz9
1qaiDHuWnUSifz6SGpWr
-----END CERTIFICATE-----`)
	b, _ := pem.Decode(mockCertbytes)
	x509cert, _ := x509.ParseCertificate(b.Bytes)
	return mockCertbytes, x509cert
}
