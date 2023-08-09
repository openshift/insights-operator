package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/stretchr/testify/assert"
)

type MockProcessingStatusClient struct {
	err      error
	response *http.Response
}

func (m *MockProcessingStatusClient) GetWithPathParams(_ context.Context, _, _ string) (*http.Response, error) {
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
			expectedErr:       nil,
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
			mockConfCtrl := &config.Controller{
				ReportPullingDelay: 10 * time.Millisecond,
			}
			processed, err := wasDataProcessed(context.Background(), &tt.mockClient, "empty", mockConfCtrl)
			assert.Equal(t, tt.expectedErr, err)
			assert.Equal(t, tt.expectedProcessed, processed)
		})
	}
}
