package insightsreport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/controllerstatus"
	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/stretchr/testify/assert"
)

func Test_readInsightsReport(t *testing.T) {
	client := mockInsightsClient{
		clusterVersion: &v1.ClusterVersion{
			Spec: v1.ClusterVersionSpec{
				ClusterID: v1.ClusterID("0000 0000 0000 0000"),
			},
		},
		metricsName: "yeet",
	}
	tests := []struct {
		name                          string
		testController                *Controller
		report                        types.SmartProxyReport
		expectedActiveRecommendations []types.InsightsRecommendation
		expectedHealthStatus          healthStatusCounts
		expectedGatherTime            string
	}{
		{
			name: "basic test with all rules enabled",
			testController: &Controller{
				configurator: config.NewMockSecretConfigurator(&config.Controller{
					DisableInsightsAlerts: false,
				}),
				client: &client,
			},
			report: types.SmartProxyReport{
				Data: []types.RuleWithContentResponse{
					{
						RuleID:      "ccx.dev.magic.recommendation",
						Description: "test rule description 1",
						Disabled:    false,
						TotalRisk:   2,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 1",
						},
					},
					{
						RuleID:      "ccx.dev.super.recommendation",
						Description: "test rule description 2",
						Disabled:    false,
						TotalRisk:   1,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 2",
						},
					},
					{
						RuleID:      "ccx.dev.cool.recommendation",
						Description: "test rule description 3",
						Disabled:    false,
						TotalRisk:   3,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 3",
						},
					},
					{
						RuleID:      "ccx.dev.ultra.recommendation",
						Description: "test rule description 4",
						Disabled:    false,
						TotalRisk:   1,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 4",
						},
					},
				},

				Meta: types.ReportResponseMeta{
					GatheredAt: types.Timestamp("2022-06-22T15:54:26Z"),
					Count:      4,
				},
			},
			expectedActiveRecommendations: []types.InsightsRecommendation{
				{
					RuleID:      "ccx.dev.magic.recommendation",
					ErrorKey:    "test error key 1",
					Description: "test rule description 1",
					TotalRisk:   2,
				},
				{
					RuleID:      "ccx.dev.super.recommendation",
					ErrorKey:    "test error key 2",
					Description: "test rule description 2",
					TotalRisk:   1,
				},
				{
					RuleID:      "ccx.dev.cool.recommendation",
					ErrorKey:    "test error key 3",
					Description: "test rule description 3",
					TotalRisk:   3,
				},
				{
					RuleID:      "ccx.dev.ultra.recommendation",
					ErrorKey:    "test error key 4",
					Description: "test rule description 4",
					TotalRisk:   1,
				},
			},
			expectedHealthStatus: healthStatusCounts{
				critical:  0,
				important: 1,
				low:       2,
				moderate:  1,
				total:     4,
			},
			expectedGatherTime: "2022-06-22 15:54:26 +0000 UTC",
		},
		{
			name: "basic test with some rules disabled",
			testController: &Controller{
				configurator: config.NewMockSecretConfigurator(&config.Controller{
					DisableInsightsAlerts: false,
				}),
				client: &client,
			},
			report: types.SmartProxyReport{
				Data: []types.RuleWithContentResponse{
					{
						RuleID:      "ccx.dev.magic.recommendation",
						Description: "test rule description 1",
						Disabled:    false,
						TotalRisk:   2,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 1",
						},
					},
					{
						RuleID:      "ccx.dev.super.recommendation",
						Description: "test rule description 2",
						Disabled:    true,
						TotalRisk:   1,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 2",
						},
					},
					{
						RuleID:      "ccx.dev.cool.recommendation",
						Description: "test rule description 3",
						Disabled:    false,
						TotalRisk:   3,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 3",
						},
					},
					{
						RuleID:      "ccx.dev.ultra.recommendation",
						Description: "test rule description 4",
						Disabled:    true,
						TotalRisk:   1,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 4",
						},
					},
				},

				Meta: types.ReportResponseMeta{
					GatheredAt: types.Timestamp("2022-06-22T15:54:26Z"),
					Count:      4,
				},
			},
			expectedActiveRecommendations: []types.InsightsRecommendation{
				{
					RuleID:      "ccx.dev.magic.recommendation",
					ErrorKey:    "test error key 1",
					Description: "test rule description 1",
					TotalRisk:   2,
				},
				{
					RuleID:      "ccx.dev.cool.recommendation",
					ErrorKey:    "test error key 3",
					Description: "test rule description 3",
					TotalRisk:   3,
				},
			},
			expectedHealthStatus: healthStatusCounts{
				critical:  0,
				important: 1,
				low:       0,
				moderate:  1,
				total:     2,
			},
			expectedGatherTime: "2022-06-22 15:54:26 +0000 UTC",
		},
		{
			name: "Insights recommendations as alerts are disabled => no active recommendations",
			testController: &Controller{
				configurator: config.NewMockSecretConfigurator(&config.Controller{
					DisableInsightsAlerts: true,
				}),
				client: &client,
			},
			report: types.SmartProxyReport{
				Data: []types.RuleWithContentResponse{
					{
						RuleID:      "ccx.dev.magic.recommendation",
						Description: "test rule description 1",
						Disabled:    false,
						TotalRisk:   2,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 1",
						},
					},
					{
						RuleID:      "ccx.dev.super.recommendation",
						Description: "test rule description 2",
						Disabled:    true,
						TotalRisk:   1,
						TemplateData: map[string]interface{}{
							"error_key": "test error key 2",
						},
					},
				},

				Meta: types.ReportResponseMeta{
					GatheredAt: types.Timestamp("2022-06-22T15:54:26Z"),
					Count:      2,
				},
			},
			expectedActiveRecommendations: []types.InsightsRecommendation{},
			expectedHealthStatus: healthStatusCounts{
				critical:  0,
				important: 0,
				low:       0,
				moderate:  1,
				total:     1,
			},
			expectedGatherTime: "2022-06-22 15:54:26 +0000 UTC",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			activeRecommendations, healthStatus := tc.testController.readInsightsReport(tc.report)
			assert.Equal(t, tc.expectedActiveRecommendations, activeRecommendations)
			assert.Equal(t, tc.expectedHealthStatus, healthStatus)
		})
	}
}

func Test_extractErrorKeyFromRuleData(t *testing.T) {
	testRuleID := "test-rule-id"
	tests := []struct {
		name             string
		ruleResponse     types.RuleWithContentResponse
		expectedErrorKey string
		expectedError    error
	}{
		{
			name: "Valid Rule response with some error key",
			ruleResponse: types.RuleWithContentResponse{
				TemplateData: map[string]interface{}{
					"error_key": "ccx_rules_ocp.external.rules.empty_prometheus_db_volume.report",
				},
			},
			expectedErrorKey: "ccx_rules_ocp.external.rules.empty_prometheus_db_volume.report",
			expectedError:    nil,
		},
		{
			name: "Rule response with empty TemplateData",
			ruleResponse: types.RuleWithContentResponse{
				RuleID:       types.RuleID(testRuleID),
				TemplateData: nil,
			},
			expectedErrorKey: "",
			expectedError:    fmt.Errorf("unable to convert the TemplateData of rule \"%s\" in an Insights report to a map", testRuleID),
		},
		{
			name: "Rule response with wrong TemplateData",
			ruleResponse: types.RuleWithContentResponse{
				RuleID: types.RuleID(testRuleID),
				TemplateData: map[string]interface{}{
					"no_error_key": "lorem ipsum",
				},
			},
			expectedErrorKey: "",
			expectedError:    fmt.Errorf("templateData of rule \"%s\" does not contain error_key", testRuleID),
		},
		{
			name: "Rule response with wrong error_key type",
			ruleResponse: types.RuleWithContentResponse{
				RuleID: types.RuleID(testRuleID),
				TemplateData: map[string]interface{}{
					"error_key": 1,
				},
			},
			expectedErrorKey: "",
			expectedError:    fmt.Errorf("the error_key of TemplateData of rule \"%s\" is not a string", testRuleID),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorKey, err := extractErrorKeyFromRuleData(tt.ruleResponse)
			assert.Equal(t, tt.expectedErrorKey, errorKey)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestPullReportTechpreview(t *testing.T) {
	tests := []struct {
		name          string
		report        *types.InsightsAnalysisReport
		statusCode    int
		conf          *config.Controller
		statusSummary controllerstatus.Summary
		mockClientErr error
		expectedErr   error
	}{
		{
			name: "Insights Analysis Report retrieved",
			report: &types.InsightsAnalysisReport{
				ClusterID: "test-cluster-ID",
				RequestID: "test-request-ID",
				Recommendations: []types.Recommendation{
					{
						ErrorKey:    "test-error-key-1",
						Description: "lorem ipsum description",
						TotalRisk:   1,
						RuleFQDN:    "test.err.key1",
					},
					{
						ErrorKey:    "test-error-key-2",
						Description: "lorem ipsum description",
						TotalRisk:   2,
						RuleFQDN:    "test.err.key2",
					},
					{
						ErrorKey:    "test-error-key-3",
						Description: "lorem ipsum description",
						TotalRisk:   3,
						RuleFQDN:    "test.err.key3",
					},
				},
			},
			statusCode: http.StatusOK,
			conf: &config.Controller{
				ReportEndpointTechPreview: "non-empty-endpoint",
			},
			statusSummary: controllerstatus.Summary{
				Healthy: true,
			},
			mockClientErr: nil,
			expectedErr:   nil,
		},
		{
			name:       "Empty report endpoint",
			report:     nil,
			statusCode: 0,
			conf: &config.Controller{
				ReportEndpointTechPreview: "",
			},
			statusSummary: controllerstatus.Summary{
				Healthy: true,
			},
			mockClientErr: nil,
			expectedErr:   nil,
		},
		{
			name:       "Insights Analysis Report not retrieved, because of error",
			report:     nil,
			statusCode: 0,
			conf: &config.Controller{
				ReportEndpointTechPreview: "non-empty-endpoint",
			},
			statusSummary: controllerstatus.Summary{
				Healthy: false,
			},
			mockClientErr: fmt.Errorf("test error"),
			expectedErr:   fmt.Errorf("test error"),
		},
		{
			name:       "Insights Analysis Report not retrieved, because of HTTP 404 response",
			report:     nil,
			statusCode: http.StatusNotFound,
			conf: &config.Controller{
				ReportEndpointTechPreview: "non-empty-endpoint",
			},
			statusSummary: controllerstatus.Summary{
				Healthy: false,
			},
			mockClientErr: nil,
			expectedErr:   fmt.Errorf("Failed to download the latest report: HTTP 404 Not Found"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.report)
			assert.NoError(t, err)
			testController := Controller{
				client: &mockInsightsClient{
					response: http.Response{
						StatusCode: tt.statusCode,
						Status:     http.StatusText(tt.statusCode),
						Body:       io.NopCloser(bytes.NewReader(data)),
					},
					err: tt.mockClientErr,
				},
				configurator: &config.MockSecretConfigurator{
					Conf: tt.conf,
				},
				StatusController: controllerstatus.New("test-insightsreport"),
			}

			report, err := testController.PullReportTechpreview("test-request-id")
			assert.Equal(t, tt.expectedErr, err)
			summary, _ := testController.StatusController.CurrentStatus()
			assert.Equal(t, tt.statusSummary.Healthy, summary.Healthy)
			if report == nil || report.Recommendations == nil {
				return
			}
			assert.Equal(t, tt.report.Recommendations, report.Recommendations)
			assert.Equal(t, tt.report.ClusterID, report.ClusterID)
			assert.Equal(t, tt.report.RequestID, report.RequestID)
		})
	}
}

type mockInsightsClient struct {
	clusterVersion *v1.ClusterVersion
	metricsName    string
	response       http.Response
	err            error
}

func (c *mockInsightsClient) GetClusterVersion() (*v1.ClusterVersion, error) {
	return c.clusterVersion, nil
}

func (c *mockInsightsClient) IncrementRecvReportMetric(_ int) {
}

func (c *mockInsightsClient) RecvReport(_ context.Context, _ string) (*http.Response, error) {
	return nil, nil
}

func (c *mockInsightsClient) GetWithPathParams(ctx context.Context, endpoint, requestID string) (*http.Response, error) {
	return &c.response, c.err
}
