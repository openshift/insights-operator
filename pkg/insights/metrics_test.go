package insights

import (
	"testing"

	v1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
)

func Test_CreateInsightsAdvisorLink(t *testing.T) {
	tests := []struct {
		name        string
		clusterID   v1.ClusterID
		ruleID      string
		errorKey    string
		expectedURL string
	}{
		{
			name:      "basic link creation",
			clusterID: v1.ClusterID("test-cluster-id"),
			ruleID:    "ccx.dev.magic.recommendation",
			errorKey:  "ERROR_KEY_1",
			expectedURL: "https://console.redhat.com/openshift/insights/advisor/clusters/" +
				"test-cluster-id?first=ccx.dev.magic.recommendation%7CERROR_KEY_1",
		},
		{
			name:      "link with special characters in error key",
			clusterID: v1.ClusterID("cluster-123"),
			ruleID:    "ccx.rules.test",
			errorKey:  "ERROR_KEY_WITH_UNDERSCORE",
			expectedURL: "https://console.redhat.com/openshift/insights/advisor/clusters/" +
				"cluster-123?first=ccx.rules.test%7CERROR_KEY_WITH_UNDERSCORE",
		},
		{
			name:      "empty cluster ID",
			clusterID: v1.ClusterID(""),
			ruleID:    "ccx.test.rule",
			errorKey:  "ERROR_KEY",
			expectedURL: "https://console.redhat.com/openshift/insights/advisor/clusters" +
				"?first=ccx.test.rule%7CERROR_KEY",
		},
		{
			name:      "UUID cluster ID",
			clusterID: v1.ClusterID("550e8400-e29b-41d4-a716-446655440000"),
			ruleID:    "ccx.ocp.rule",
			errorKey:  "EXAMPLE_ERROR",
			expectedURL: "https://console.redhat.com/openshift/insights/advisor/clusters/" +
				"550e8400-e29b-41d4-a716-446655440000?first=ccx.ocp.rule%7CEXAMPLE_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CreateInsightsAdvisorLink(tt.clusterID, tt.ruleID, tt.errorKey)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedURL, result)
		})
	}
}

func Test_totalRiskToStr(t *testing.T) {
	tests := []struct {
		name      string
		totalRisk int32
		expected  string
	}{
		{
			name:      "risk level 1 - Low",
			totalRisk: 1,
			expected:  "Low",
		},
		{
			name:      "risk level 2 - Moderate",
			totalRisk: 2,
			expected:  "Moderate",
		},
		{
			name:      "risk level 3 - Important",
			totalRisk: 3,
			expected:  "Important",
		},
		{
			name:      "risk level 4 - Critical",
			totalRisk: 4,
			expected:  "Critical",
		},
		{
			name:      "invalid risk level 0",
			totalRisk: 0,
			expected:  "Invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := totalRiskToStr(tt.totalRisk)
			assert.Equal(t, tt.expected, result)
		})
	}
}
