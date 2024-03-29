package clusterconfig

import (
	"bufio"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherOpenshiftSDNLogs(t *testing.T) {
	// Unit Tests
	testCases := []struct {
		name     string
		logline  string
		expected string
	}{
		{
			name:     "Logs with no matches do not return errors",
			logline:  "mock logline",
			expected: "",
		},
		{
			name:     "Logs with OnEndpointsUpdate expected string matches successfully",
			logline:  "{text before} Got OnEndpointsUpdate for unknown Endpoints {text after}",
			expected: "Got OnEndpointsUpdate for unknown Endpoints",
		},
		{
			name:     "Logs with OnEndpointsDelete expected string matches successfully",
			logline:  "{text before} Got OnEndpointsDelete for unknown Endpoints {text after}",
			expected: "Got OnEndpointsDelete for unknown Endpoints",
		},
		{
			name:     "Logs with 'Unable to update proxy firewall' expected string matches successfully",
			logline:  "{text before} Unable to update proxy firewall for policy {text after}",
			expected: "Unable to update proxy firewall for policy",
		},
		{
			name:     "Logs with 'Failed to update proxy firewall' expected string matches successfully",
			logline:  "{text before} Failed to update proxy firewall for policy {text after}",
			expected: "Failed to update proxy firewall for policy",
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			msgFilter := getGatherOpenshiftSDNLogsMessageFilter()

			// When
			test, err := common.FilterLogFromScanner(
				bufio.NewScanner(strings.NewReader(
					tc.logline,
				)),
				msgFilter.MessagesToSearch,
				msgFilter.IsRegexSearch,
				nil)

			// Assert
			assert.NoError(t, err)
			assert.Contains(t, test, tc.expected)
		})
	}
}
