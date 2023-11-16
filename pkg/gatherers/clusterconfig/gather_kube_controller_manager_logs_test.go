package clusterconfig

import (
	"bufio"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherKubeControllerManagerLogs(t *testing.T) {
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
			name:     "Logs with 'Internal error occurred: error resolving resource' string matches successfully",
			logline:  "{text before} Internal error occurred: error resolving resource {text after}",
			expected: "Internal error occurred: error resolving resource",
		},
		{
			name:     "Logs with 'syncing garbage collector with updated resources from discovery' string matches successfully",
			logline:  "{text before} syncing garbage collector with updated resources from discovery {text after}",
			expected: "syncing garbage collector with updated resources from discovery",
		},
	}

	// Given
	msgFilter := getKubeControllerManagerLogsMessagesFilter()
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
