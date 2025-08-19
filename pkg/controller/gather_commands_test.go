package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockProcessingStatusClient struct {
	err      error
	response *http.Response
}

func (m *MockProcessingStatusClient) GetWithPathParam(_ context.Context, _, _ string, _ bool) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.response, nil
}

func TestWasDataProcessed(t *testing.T) {
	tests := []struct {
		name              string
		mockClient        MockProcessingStatusClient
		expectedProcessed bool
		expectedErr       error
	}{
		{
			name: "no response with error",
			mockClient: MockProcessingStatusClient{
				response: nil,
				err:      fmt.Errorf("no response received"),
			},
			expectedProcessed: false,
			expectedErr:       fmt.Errorf("no response received"),
		},
		{
			name: "HTTP 404 response and no body",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       http.NoBody,
				},
				err: nil,
			},
			expectedProcessed: false,
			expectedErr:       fmt.Errorf("HTTP status message: %s", http.StatusText(http.StatusNotFound)),
		},
		{
			name: "HTTP 404 response and existing body",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("test message")),
				},
				err: nil,
			},
			expectedProcessed: false,
			expectedErr:       fmt.Errorf("HTTP status message: %s", http.StatusText(http.StatusNotFound)),
		},
		{
			name: "data not processed",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{\"cluster\":\"test-uid\",\"status\":\"unknown\"}")),
				},
				err: nil,
			},
			expectedProcessed: false,
			expectedErr:       nil,
		},
		{
			name: "data processed",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{\"cluster\":\"test-uid\",\"status\":\"processed\"}")),
				},
				err: nil,
			},
			expectedProcessed: true,
			expectedErr:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := &config.InsightsConfiguration{
				DataReporting: config.DataReporting{
					ReportPullingDelay: 10 * time.Millisecond,
				},
			}
			processed, err := wasDataProcessed(context.Background(), &tt.mockClient, "empty", mockConfig)
			assert.Equal(t, tt.expectedErr, err)
			assert.Equal(t, tt.expectedProcessed, processed)
		})
	}
}

func TestCreateRemoteConfigConditions(t *testing.T) {
	tests := []struct {
		name                             string
		remoteConfigStatus               *gatherers.RemoteConfigStatus
		expectedRemoteConfigAvailableCon metav1.Condition
		expectedRemoteConfigValidCon     metav1.Condition
	}{
		{
			name:               "Remote Config status is nil/unknown",
			remoteConfigStatus: nil,
			expectedRemoteConfigAvailableCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationAvailable),
				Status:  metav1.ConditionUnknown,
				Reason:  status.RemoteConfNotRequestedYet,
				Message: "",
			},
			expectedRemoteConfigValidCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationValid),
				Status:  metav1.ConditionUnknown,
				Reason:  status.RemoteConfNotValidatedYet,
				Message: "",
			},
		},
		{
			name: "Remote Config status is available and valid",
			remoteConfigStatus: &gatherers.RemoteConfigStatus{
				AvailableReason: status.AsExpectedReason,
				ValidReason:     status.AsExpectedReason,
				ConfigAvailable: true,
				ConfigValid:     true,
			},
			expectedRemoteConfigAvailableCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationAvailable),
				Status:  metav1.ConditionTrue,
				Reason:  status.AsExpectedReason,
				Message: "",
			},
			expectedRemoteConfigValidCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationValid),
				Status:  metav1.ConditionTrue,
				Reason:  status.AsExpectedReason,
				Message: "",
			},
		},
		{
			name: "Remote Config status is unvailable",
			remoteConfigStatus: &gatherers.RemoteConfigStatus{
				AvailableReason: "Failed",
				ConfigAvailable: false,
				ConfigValid:     false,
				Err:             fmt.Errorf("endpoint not reachable"),
			},
			expectedRemoteConfigAvailableCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationAvailable),
				Status:  metav1.ConditionFalse,
				Reason:  "Failed",
				Message: "endpoint not reachable",
			},
			expectedRemoteConfigValidCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationValid),
				Status:  metav1.ConditionUnknown,
				Reason:  status.RemoteConfNotValidatedYet,
				Message: "",
			},
		},
		{
			name: "Remote Config status is available but invalid",
			remoteConfigStatus: &gatherers.RemoteConfigStatus{
				AvailableReason: status.AsExpectedReason,
				ValidReason:     "Invalid",
				ConfigAvailable: true,
				ConfigValid:     false,
				Err:             fmt.Errorf("cannot parse"),
			},
			expectedRemoteConfigAvailableCon: metav1.Condition{
				Type:   string(status.RemoteConfigurationAvailable),
				Status: metav1.ConditionTrue,
				Reason: status.AsExpectedReason,
			},
			expectedRemoteConfigValidCon: metav1.Condition{
				Type:    string(status.RemoteConfigurationValid),
				Status:  metav1.ConditionFalse,
				Reason:  "Invalid",
				Message: "cannot parse",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rcAvailableCon, rcValidCon := createRemoteConfigConditions(tt.remoteConfigStatus)
			assert.True(t, conditionsEqual(&rcAvailableCon, &tt.expectedRemoteConfigAvailableCon))
			assert.True(t, conditionsEqual(&rcValidCon, &tt.expectedRemoteConfigValidCon))
		})
	}
}

func conditionsEqual(a, b *metav1.Condition) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Status != b.Status {
		return false
	}
	if a.Reason != b.Reason {
		return false
	}
	if a.Message != b.Message {
		return false
	}
	if a.ObservedGeneration != b.ObservedGeneration {
		return false
	}
	return true
}

// MockConfigAggregator implements configobserver.Interface for testing
type MockConfigAggregator struct {
	storagePath string
}

func (m *MockConfigAggregator) Config() *config.InsightsConfiguration {
	return &config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			StoragePath: m.storagePath,
		},
	}
}

func (m *MockConfigAggregator) ConfigChanged() (_ <-chan struct{}, _ func()) {
	return nil, nil
}

func (m *MockConfigAggregator) Listen(_ context.Context) {}

func TestGetCustomStoragePath(t *testing.T) {
	tests := []struct {
		name         string
		mockConfig   configobserver.Interface
		dataGatherCR *insightsv1.DataGather
		expectedPath string
	}{
		{
			name:         "When both CR and ConfigMap have no storage path configured, should return empty string",
			mockConfig:   &MockConfigAggregator{storagePath: ""},
			dataGatherCR: &insightsv1.DataGather{},
			expectedPath: "",
		},
		{
			name:         "When only ConfigMap has storage path configured, should return ConfigMap path",
			mockConfig:   &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1.DataGather{},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When only CR has storage path configured, should return CR path",
			mockConfig: &MockConfigAggregator{storagePath: ""},
			dataGatherCR: &insightsv1.DataGather{
				Spec: insightsv1.DataGatherSpec{
					Storage: insightsv1.Storage{
						Type:             insightsv1.StorageTypePersistentVolume,
						PersistentVolume: insightsv1.PersistentVolumeConfig{MountPath: "/cr/path"},
					},
				},
			},
			expectedPath: "/cr/path",
		},
		{
			name:       "When CR has correct storage type but empty mount path (misconfiguration), should fall back to ConfigMap path",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1.DataGather{
				Spec: insightsv1.DataGatherSpec{
					Storage: insightsv1.Storage{
						Type:             insightsv1.StorageTypePersistentVolume,
						PersistentVolume: insightsv1.PersistentVolumeConfig{MountPath: ""},
					},
				},
			},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When both CR and ConfigMap have storage path configured, CR path should take precedence",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1.DataGather{
				Spec: insightsv1.DataGatherSpec{
					Storage: insightsv1.Storage{
						Type: insightsv1.StorageTypePersistentVolume,
						PersistentVolume: insightsv1.PersistentVolumeConfig{
							MountPath: "/cr/path",
							Claim:     insightsv1.PersistentVolumeClaimReference{Name: "test-pvc"},
						},
					},
				},
			},
			expectedPath: "/cr/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			result := getCustomStoragePath(tt.mockConfig, tt.dataGatherCR)

			// Assert
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}
