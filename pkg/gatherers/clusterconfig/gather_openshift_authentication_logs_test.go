package clusterconfig

import (
	"bufio"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherOpenshiftAuthenticationLogs(t *testing.T) {
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
			name:     "Logs with 'AuthenticationError: ' string matches successfully",
			logline:  "{text before} AuthenticationError: invalid resource name {text after}",
			expected: "AuthenticationError: invalid resource name",
		},
	}

	// Given
	msgFilter := getOpenshiftAuthenticationLogsMessagesFilter()
	var messagesRegex *regexp.Regexp
	if msgFilter.IsRegexSearch {
		messagesRegex = regexp.MustCompile(strings.Join(msgFilter.MessagesToSearch, "|"))
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			test, err := common.FilterLogFromScanner(
				bufio.NewScanner(strings.NewReader(
					tc.logline,
				)),
				msgFilter.MessagesToSearch,
				messagesRegex,
				nil)

			// Assert
			assert.NoError(t, err)
			assert.Contains(t, test, tc.expected)
		})
	}
}
