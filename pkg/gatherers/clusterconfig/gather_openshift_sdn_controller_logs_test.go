package clusterconfig

import (
	"bufio"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherOpenshiftSDNControllerLogs(t *testing.T) {
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
			name:     "'Node is not Ready' search matches successfully",
			logline:  "{text before} Node 'test' is not Ready {text after}",
			expected: "Node 'test' is not Ready",
		},
		{
			name:     "'Node may be offline' search matches successfully",
			logline:  "{text before} Node 'test' may be offline... retrying {text after}",
			expected: "Node 'test' may be offline... retrying",
		},
		{
			name:     "'Node is offline' search matches successfully",
			logline:  "{text before} Node 'test' is offline {text after}",
			expected: "Node 'test' is offline",
		},
		{
			name:     "'Node is back online' search matches successfully",
			logline:  "{text before} Node 'test' is back online {text after}",
			expected: "Node 'test' is back online",
		},
	}

	// Given
	msgFilter := getSDNControllerLogsMessagesFilter()
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
