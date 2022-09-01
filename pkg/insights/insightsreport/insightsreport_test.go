package insightsreport

import (
	"fmt"
	"testing"

	"github.com/openshift/insights-operator/pkg/insights/types"
	"github.com/stretchr/testify/assert"
)

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
			expectedError:    fmt.Errorf("TemplateData of rule \"%s\" does not contain error_key", testRuleID),
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
			expectedError:    fmt.Errorf("The error_key of TemplateData of rule \"%s\" is not a string", testRuleID),
		},
	}

	for _, tt := range tests {
		errorKey, err := extractErrorKeyFromRuleData(tt.ruleResponse)
		assert.Equal(t, tt.expectedErrorKey, errorKey)
		assert.Equal(t, tt.expectedError, err)
	}
}
