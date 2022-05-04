package clustertransfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_ClusterTransfer_PullSecretUpdate(t *testing.T) {
	kube := kubefake.NewSimpleClientset()
	coreClient := kube.CoreV1()
	ctController := New(context.TODO(), coreClient, nil, nil)

	tests := []struct {
		name          string
		defaultData   []byte
		dataToUpdate  []byte
		updatedResult []byte
		updating      bool
	}{
		{
			name:          "Updating cloud.openshift.com auth attributes and quay.io email",
			defaultData:   []byte(`{"auths":{"cloud.openshift.com":{"auth":"eyJSb2xlIjoiwMjMsImlhdCI6MTY0MTg5MDAyM30","email":"test@test.com"},"quay.io":{"auth":"l3YM6DX9frFXhswvfuBv__dhrpDACa4F0E","email":"test-quay@test.com"}}}`),            // nolint: lll
			dataToUpdate:  []byte(`{"auths":{"cloud.openshift.com":{"auth":"eyJSb2xlIjoiwMjMsImlhdCI6MTY0MTg5MDAyM30==","email":"updated@updated.com"},"quay.io":{"email":"test-quay@updated.com"}}}`),                                             // nolint: lll
			updatedResult: []byte(`{"auths":{"cloud.openshift.com":{"auth":"eyJSb2xlIjoiwMjMsImlhdCI6MTY0MTg5MDAyM30==","email":"updated@updated.com"},"quay.io":{"auth":"l3YM6DX9frFXhswvfuBv__dhrpDACa4F0E","email":"test-quay@updated.com"}}}`), // nolint: lll
			updating:      true,
		},
		{
			name:          "Updating cloud.openshift.com token and add quay.io auth",
			defaultData:   []byte(`{"auths":{"cloud.openshift.com":{"auth":"xyz","email":"test@test.com"}}}`),
			dataToUpdate:  []byte(`{"auths":{"cloud.openshift.com":{"auth":"abcde.123456"},"quay.io":{"auth":"l3YM6DX9frFXhswvfuBv__dhrpDACa4F0E","email":"test-quay@updated.com"}}}`),                         // nolint: lll
			updatedResult: []byte(`{"auths":{"cloud.openshift.com":{"auth":"abcde.123456","email":"test@test.com"},"quay.io":{"auth":"l3YM6DX9frFXhswvfuBv__dhrpDACa4F0E","email":"test-quay@updated.com"}}}`), // nolint: lll
			updating:      true,
		},
		{
			name:          "Updating only cloud.openshift.com token",
			defaultData:   []byte(`{"auths":{"cloud.openshift.com":{"auth":"xyz","email":"test@test.com"},"registry.redhat.io":{"auth":"NjQ5MzY0N3x1aGMtMVl0Vnd2WmdYTENibkdCT2piTWtiRFY3bmdlOmV5SmhiR2NpT2lKU1V6VXhNaUo5LmV5SnpkV0lp","email":"test@test.org"}},"HttpHeaders":{"header1":"value1"}}`), // nolint: lll
			dataToUpdate:  []byte(`{"auths":{"cloud.openshift.com":{"auth":"abcde.123456"}}}`),
			updatedResult: []byte(`{"HttpHeaders":{"header1":"value1"},"auths":{"cloud.openshift.com":{"auth":"abcde.123456","email":"test@test.com"},"registry.redhat.io":{"auth":"NjQ5MzY0N3x1aGMtMVl0Vnd2WmdYTENibkdCT2piTWtiRFY3bmdlOmV5SmhiR2NpT2lKU1V6VXhNaUo5LmV5SnpkV0lp","email":"test@test.org"}}}`), // nolint: lll
			updating:      true,
		},
		{
			name:          "No update required",
			defaultData:   []byte(`{"auths":{"cloud.openshift.com":{"auth":"xyz","email":"test@test.com"},"registry.redhat.io":{"auth":"NjQ5MzY0N3x1aGMtMVl0Vnd2WmdYTENibkdCT2piTWtiRFY3bmdlOmV5SmhiR2NpT2lKU1V6VXhNaUo5LmV5SnpkV0lp","email":"test@test.org"}},"HttpHeaders":{"header1":"value1"}}`), // nolint: lll
			dataToUpdate:  []byte(`{"auths":{"cloud.openshift.com":{"email":"test@test.com","auth":"xyz"}}}`),
			updatedResult: nil,
			updating:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pullSecret, err := createPullSecret(coreClient, tt.defaultData)
			assert.NoError(t, err)

			ps, err := ctController.getPullSecret()
			assert.NoError(t, err, "can't get pull-secret")
			assert.Equal(t, tt.defaultData, ps.Data[v1.DockerConfigJsonKey])

			updating, err := ctController.isUpdateRequired(tt.dataToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.updating, updating, "unexpected update requirement")

			if updating {
				err = ctController.updatePullSecret(tt.dataToUpdate)
				assert.NoError(t, err, "can't update pull-secret data")

				ps, err = ctController.getPullSecret()
				assert.NoError(t, err, "can't get pull-secret")
				assert.Equal(t, tt.updatedResult, ps.Data[v1.DockerConfigJsonKey], "secret was not updated correctly")
			}

			err = coreClient.Secrets("openshift-config").Delete(context.TODO(), pullSecret.Name, metav1.DeleteOptions{})
			assert.NoError(t, err, "can't delete pull-secret")
		})
	}
}

func Test_ClusterTransfer_RequestDataAndUpdateSecret(t *testing.T) {
	tests := []struct {
		name                          string
		pullSecretDataFilePath        string
		clusterTransferDataFilePath   string
		expectedSummary               controllerstatus.Summary
		updatedPullSecretDataFilePath string
	}{
		{
			name:                        "more accepted cluster transfers do not change pull-secret value",
			pullSecretDataFilePath:      "test-data/test-pull-secret.json",
			clusterTransferDataFilePath: "test-data/more-cluster-transfers.json",
			expectedSummary: controllerstatus.Summary{
				Operation: controllerstatus.PullingClusterTransfer,
				Healthy:   true,
				Reason:    "MoreAcceptedClusterTransfers",
				Count:     1,
				Message:   "there are more accepted cluster transfers. The pull-secret will not be updated!"},
			// no update expected so the same file
			updatedPullSecretDataFilePath: "test-data/test-pull-secret.json",
		},
		{
			name:                        "accepted cluster transfer updates pull-secret value",
			pullSecretDataFilePath:      "test-data/test-pull-secret.json",
			clusterTransferDataFilePath: "test-data/accepted-cluster-transfer.json",
			expectedSummary: controllerstatus.Summary{
				Operation: controllerstatus.PullingClusterTransfer,
				Healthy:   true,
				Reason:    "PullSecretUpdated",
				Count:     1,
				Message:   "pull-secret successfully updated"},
			updatedPullSecretDataFilePath: "test-data/updated-pull-secret.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kube := kubefake.NewSimpleClientset()
			coreClient := kube.CoreV1()
			mockConfig := &config.MockConfigurator{
				Conf: &config.Controller{
					OCMConfig: config.OCMConfig{ClusterTransferEndpoint: "/cluster_transfer"},
				},
			}
			ctResponse, err := loadDataFromFile(tt.clusterTransferDataFilePath)
			assert.NoError(t, err)
			mockClient := &MockClusterTransferClient{data: string(ctResponse)}

			ctController := New(context.Background(), coreClient, mockConfig, mockClient)
			_, err = createPullSecretFromFile(coreClient, tt.pullSecretDataFilePath)
			assert.NoError(t, err)

			ctController.requestDataAndUpdateSecret(mockConfig.Conf.OCMConfig.ClusterTransferEndpoint)
			summary, ok := ctController.CurrentStatus()
			assert.True(t, ok, "unexpected summary")
			assert.EqualValues(t, tt.expectedSummary, summary)

			// check pull-secret value
			expectedPSData, err := loadDataFromFile(tt.updatedPullSecretDataFilePath)
			assert.NoError(t, err)
			ps, err := ctController.getPullSecret()
			assert.NoError(t, err)
			assert.EqualValues(t, expectedPSData, ps.Data[v1.DockerConfigJsonKey])

			// delete pull-secret
			err = coreClient.Secrets("openshift-config").Delete(context.TODO(), ps.Name, metav1.DeleteOptions{})
			assert.NoError(t, err, "can't delete pull-secret")
		})
	}
}

func createPullSecretFromFile(client corev1client.CoreV1Interface, filePath string) (*v1.Secret, error) {
	data, err := loadDataFromFile(filePath)
	if err != nil {
		return nil, err
	}

	return createPullSecret(client, data)
}

func createPullSecret(client corev1client.CoreV1Interface, data []byte) (*v1.Secret, error) {
	pullSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pull-secret", Namespace: "openshift-config"},
		Type:       v1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			v1.DockerConfigJsonKey: data,
		},
	}

	ps, err := client.Secrets("openshift-config").Create(context.TODO(), pullSecret, metav1.CreateOptions{})
	return ps, err
}

func loadDataFromFile(filePath string) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

type MockClusterTransferClient struct {
	data string
	err  error
}

func (s *MockClusterTransferClient) RecvClusterTransfer(endpoint string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}

	if strings.HasSuffix(endpoint, "cluster_transfer") {
		return []byte(s.data), nil
	}

	return nil, fmt.Errorf("endpoint not supported")
}
