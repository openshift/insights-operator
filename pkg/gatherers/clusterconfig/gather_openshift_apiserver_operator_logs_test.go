package clusterconfig

import (
	"bufio"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherOpenShiftAPIServerOperatorLogs(t *testing.T) {
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
			name:     "Logs with 'the server has received too many ...' string matches successfully",
			logline:  "{text before} the server has received too many requests and has asked us {text after}",
			expected: "the server has received too many requests and has asked us",
		},
		{
			name:     "Logs with 'because serving request ...' string matches successfully",
			logline:  "{text before} because serving request timed out and response had been started {text after}",
			expected: "because serving request timed out and response had been started",
		},
	}

	// Given
	msgFilter := getAPIServerOperatorLogsMessagesFilter()

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
