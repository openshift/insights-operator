package clusterconfig

import (
	"bufio"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherSAPVsystemIptablesLogs(t *testing.T) {
	// Unit Tests
	testCases := []struct {
		name     string
		logline  string
		expected string
	}{
		{
			name:     "No log line matches the messages to search",
			logline:  "mock logline",
			expected: "",
		},
		{
			name:     "Logs with 'can't initialize iptables table' string matches successfully",
			logline:  "{text before} can't initialize iptables table {text after}",
			expected: "can't initialize iptables table",
		},
	}

	// Given
	msgFilter := getSAPLicenseManagementLogsMessageFilter()

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			test, err := common.FilterLogFromScanner(
				bufio.NewScanner(strings.NewReader(
					tc.logline,
				)),
				common.WithSubstringFilter(msgFilter.MessagesToSearch),
				nil)

			// Assert
			assert.NoError(t, err)
			assert.Contains(t, test, tc.expected)
		})
	}
}
