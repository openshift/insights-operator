package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	insightsv1alpha2 "github.com/openshift/api/insights/v1alpha2"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/controller/status"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockProcessingStatusClient struct {
	err          error
	response     *http.Response
	responseBody string
}

func (m *MockProcessingStatusClient) GetWithPathParam(_ context.Context, _, _ string, _ bool) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}

	// Create a fresh body reader for each call to handle deferred close in the retry loop
	if m.responseBody != "" {
		m.response.Body = io.NopCloser(strings.NewReader(m.responseBody))
	}

	return m.response, nil
}

func TestWasDataProcessed(t *testing.T) {
	tests := []struct {
		name              string
		mockClient        MockProcessingStatusClient
		expectedProcessed bool
		expectError       bool
		errorContains     string
	}{
		{
			name: "no response with error",
			mockClient: MockProcessingStatusClient{
				response: nil,
				err:      fmt.Errorf("no response received"),
			},
			expectedProcessed: false,
			expectError:       true,
			errorContains:     "failed to check processing status after 3 retries",
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
			expectError:       true,
			errorContains:     "HTTP status message: Not Found",
		},
		{
			name: "HTTP 404 response and existing body",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusNotFound,
				},
				responseBody: "test message",
				err:          nil,
			},
			expectedProcessed: false,
			expectError:       true,
			errorContains:     "HTTP status message: Not Found",
		},
		{
			name: "data not processed",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusOK,
				},
				responseBody: "{\"cluster\":\"test-uid\",\"status\":\"unknown\"}",
				err:          nil,
			},
			expectedProcessed: false,
			expectError:       true,
			errorContains:     "data processing status is \"unknown\" after 3 retries, stopping poll",
		},
		{
			name: "data processed",
			mockClient: MockProcessingStatusClient{
				response: &http.Response{
					StatusCode: http.StatusOK,
				},
				responseBody: "{\"cluster\":\"test-uid\",\"status\":\"processed\"}",
				err:          nil,
			},
			expectedProcessed: true,
			expectError:       false,
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
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedProcessed, processed)
		})
	}
}

func TestProcessSuccessfulResponse(t *testing.T) {
	tests := []struct {
		name                 string
		respBody             io.ReadCloser
		initialStatusCounter int
		delay                time.Duration
		expectedDone         bool
		expectError          bool
		expectedErrorMsg     string
		expectedCounter      int
	}{
		{
			name:            "nil response body",
			respBody:        nil,
			delay:           10 * time.Millisecond,
			expectedDone:    false,
			expectError:     false,
			expectedCounter: 0,
		},
		{
			name:            "http.NoBody",
			respBody:        http.NoBody,
			delay:           10 * time.Millisecond,
			expectedDone:    false,
			expectError:     false,
			expectedCounter: 0,
		},
		{
			name:             "invalid JSON",
			respBody:         io.NopCloser(strings.NewReader("invalid json")),
			delay:            10 * time.Millisecond,
			expectedDone:     false,
			expectError:      true,
			expectedErrorMsg: "invalid character",
			expectedCounter:  0,
		},
		{
			name:                 "status not processed and counter below max - should call statusRetry",
			respBody:             io.NopCloser(strings.NewReader(`{"cluster":"test-uid","status":"unknown"}`)),
			initialStatusCounter: 0,
			delay:                10 * time.Millisecond,
			expectedDone:         false,
			expectError:          false,
			expectedCounter:      1,
		},
		{
			name:                 "status not processed and counter at max - should return error from statusRetry",
			respBody:             io.NopCloser(strings.NewReader(`{"cluster":"test-uid","status":"unknown"}`)),
			initialStatusCounter: numberOfStatusQueryRetries,
			delay:                10 * time.Millisecond,
			expectedDone:         false,
			expectError:          true,
			expectedErrorMsg:     "data processing status is \"unknown\" after 3 retries",
			expectedCounter:      numberOfStatusQueryRetries,
		},
		{
			name:            "status processed",
			respBody:        io.NopCloser(strings.NewReader(`{"cluster":"test-uid","status":"processed"}`)),
			delay:           10 * time.Millisecond,
			expectedDone:    true,
			expectError:     false,
			expectedCounter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := retryCounter{
				status: tt.initialStatusCounter,
				max:    numberOfStatusQueryRetries,
			}

			done, err := processSuccessfulResponse(tt.respBody, &rc, tt.delay)

			assert.Equal(t, tt.expectedDone, done)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCounter, rc.status)
		})
	}
}

func TestRetryFunctions(t *testing.T) {
	tests := []struct {
		name                  string
		retryType             string // "network", "request", or "status"
		initialCounter        int
		expectError           bool
		expectedErrorContains string
		expectedCounterAfter  int
	}{
		// Network retry tests
		{
			name:                 "network retry below max",
			retryType:            "network",
			initialCounter:       0,
			expectError:          false,
			expectedCounterAfter: 1,
		},
		{
			name:                  "network retry at max",
			retryType:             "network",
			initialCounter:        numberOfStatusQueryRetries,
			expectError:           true,
			expectedErrorContains: "failed to check processing status after 3 retries",
			expectedCounterAfter:  numberOfStatusQueryRetries,
		},
		// Request retry tests
		{
			name:                 "request retry below max",
			retryType:            "request",
			initialCounter:       0,
			expectError:          false,
			expectedCounterAfter: 1,
		},
		{
			name:                  "request retry at max",
			retryType:             "request",
			initialCounter:        numberOfStatusQueryRetries,
			expectError:           true,
			expectedErrorContains: "HTTP status message: Not Found",
			expectedCounterAfter:  numberOfStatusQueryRetries,
		},
		// Status retry tests
		{
			name:                 "status retry below max",
			retryType:            "status",
			initialCounter:       0,
			expectError:          false,
			expectedCounterAfter: 1,
		},
		{
			name:                  "status retry at max",
			retryType:             "status",
			initialCounter:        numberOfStatusQueryRetries,
			expectError:           true,
			expectedErrorContains: "data processing status is \"unknown\" after 3 retries, stopping poll",
			expectedCounterAfter:  numberOfStatusQueryRetries,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := retryCounter{max: numberOfStatusQueryRetries}
			delay := 10 * time.Millisecond

			var err error
			switch tt.retryType {
			case "network":
				rc.network = tt.initialCounter
				err = networkRetry(&rc, fmt.Errorf("network timeout"), delay)
				assert.Equal(t, tt.expectedCounterAfter, rc.network)
			case "request":
				rc.request = tt.initialCounter
				err = requestRetry(&rc, http.StatusNotFound, delay)
				assert.Equal(t, tt.expectedCounterAfter, rc.request)
			case "status":
				rc.status = tt.initialCounter
				done, statusErr := statusRetry(&rc, "unknown", delay)
				err = statusErr
				assert.False(t, done)
				assert.Equal(t, tt.expectedCounterAfter, rc.status)
			}

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorContains)
			} else {
				assert.NoError(t, err)
			}
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
		dataGatherCR *insightsv1alpha2.DataGather
		expectedPath string
	}{
		{
			name:         "When dataGatherCR is nil, should return ConfigMap path",
			mockConfig:   &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: nil,
			expectedPath: "/configmap/path",
		},
		{
			name:         "When both CR and ConfigMap have no storage path configured, should return empty string",
			mockConfig:   &MockConfigAggregator{storagePath: ""},
			dataGatherCR: &insightsv1alpha2.DataGather{},
			expectedPath: "",
		},
		{
			name:         "When only ConfigMap has storage path configured, should return ConfigMap path",
			mockConfig:   &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1alpha2.DataGather{},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When only CR has storage path configured, should return CR path",
			mockConfig: &MockConfigAggregator{storagePath: ""},
			dataGatherCR: &insightsv1alpha2.DataGather{
				Spec: insightsv1alpha2.DataGatherSpec{
					Storage: &insightsv1alpha2.Storage{
						Type:             insightsv1alpha2.StorageTypePersistentVolume,
						PersistentVolume: &insightsv1alpha2.PersistentVolumeConfig{MountPath: "/cr/path"},
					},
				},
			},
			expectedPath: "/cr/path",
		},
		{
			name:       "When CR Storage is nil, should return ConfigMap path",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1alpha2.DataGather{
				Spec: insightsv1alpha2.DataGatherSpec{
					Storage: nil,
				},
			},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When Storage type is Ephemeral, should return ConfigMap path",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1alpha2.DataGather{
				Spec: insightsv1alpha2.DataGatherSpec{
					Storage: &insightsv1alpha2.Storage{
						Type: insightsv1alpha2.StorageTypeEphemeral,
					},
				},
			},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When Storage type is PersistentVolume but PersistentVolume is nil (edge case), should return ConfigMap path",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1alpha2.DataGather{
				Spec: insightsv1alpha2.DataGatherSpec{
					Storage: &insightsv1alpha2.Storage{
						Type:             insightsv1alpha2.StorageTypePersistentVolume,
						PersistentVolume: nil, // This should not happen with validation, but defensive code handles it
					},
				},
			},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When CR has correct storage type but empty mount path (misconfiguration), should fall back to ConfigMap path",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1alpha2.DataGather{
				Spec: insightsv1alpha2.DataGatherSpec{
					Storage: &insightsv1alpha2.Storage{
						Type:             insightsv1alpha2.StorageTypePersistentVolume,
						PersistentVolume: &insightsv1alpha2.PersistentVolumeConfig{MountPath: ""},
					},
				},
			},
			expectedPath: "/configmap/path",
		},
		{
			name:       "When both CR and ConfigMap have storage path configured, CR path should take precedence",
			mockConfig: &MockConfigAggregator{storagePath: "/configmap/path"},
			dataGatherCR: &insightsv1alpha2.DataGather{
				Spec: insightsv1alpha2.DataGatherSpec{
					Storage: &insightsv1alpha2.Storage{
						Type: insightsv1alpha2.StorageTypePersistentVolume,
						PersistentVolume: &insightsv1alpha2.PersistentVolumeConfig{
							MountPath: "/cr/path",
							Claim:     insightsv1alpha2.PersistentVolumeClaimReference{Name: "test-pvc"},
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

// Test_storagePathExists tests the storagePathExists method
func Test_storagePathExists(t *testing.T) {
	tests := []struct {
		name          string
		setupPath     func() string
		cleanupPath   func(string)
		expectError   bool
		errorContains string
	}{
		{
			name: "storage path already exists",
			setupPath: func() string {
				// Create a temp directory that exists
				tmpDir := t.TempDir()
				return tmpDir
			},
			cleanupPath: func(string) {
				// TempDir cleans up automatically
			},
			expectError: false,
		},
		{
			name: "storage path does not exist and can be created",
			setupPath: func() string {
				tmpDir := t.TempDir()
				return tmpDir + "/new-storage-path"
			},
			cleanupPath: func(string) {
				// TempDir cleans up automatically
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath()
			defer tt.cleanupPath(path)

			gatherJob := &GatherJob{
				Controller: config.Controller{
					StoragePath: path,
				},
			}

			err := gatherJob.storagePathExists()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				// Verify the directory was created
				info, statErr := os.Stat(path)
				assert.NoError(t, statErr, "Directory should exist after storagePathExists")
				assert.True(t, info.IsDir(), "Path should be a directory")
			}
		})
	}
}
