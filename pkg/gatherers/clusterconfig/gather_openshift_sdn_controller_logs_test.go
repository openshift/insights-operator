package clusterconfig

import (
	"bufio"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherOpenshiftSDNControllerLogs(t *testing.T) {
	// Helper func
	mockLogline := func(s string) *bufio.Scanner {
		return bufio.NewScanner(strings.NewReader(s))
	}

	// Unit Tests
	tests := []struct {
		name     string
		logline  *bufio.Scanner
		expected string
	}{
		{
			name:     "No log line matches the messages to search",
			logline:  mockLogline("mock logline"),
			expected: "",
		},
		{
			name:     "'Node is not Ready' search matches successfully",
			logline:  mockLogline("Node 'test' is not Ready"),
			expected: "Node 'test' is not Ready",
		},
		{
			name:     "'Node may be offline' search matches successfully",
			logline:  mockLogline("Node 'test' may be offline... retrying"),
			expected: "Node 'test' may be offline... retrying",
		},
		{
			name:     "'Node is offline' search matches successfully",
			logline:  mockLogline("Node 'test' is offline"),
			expected: "Node 'test' is offline",
		},
		{
			name:     "'Node is back online' search matches successfully",
			logline:  mockLogline("Node 'test' is back online"),
			expected: "Node 'test' is back online",
		},
	}

	for _, unitTest := range tests {
		t.Run(unitTest.name, func(t *testing.T) {
			// Given
			msgFilter := getSDNControllerLogsMessagesFilter()

			// When
			test, err := common.FilterLogFromScanner(
				unitTest.logline,
				msgFilter.MessagesToSearch,
				msgFilter.IsRegexSearch,
				nil)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, unitTest.expected, test)
		})
	}
}
