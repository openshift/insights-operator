package clusterconfig

import (
	"bufio"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/gatherers/common"
	"github.com/stretchr/testify/assert"
)

func Test_GatherOpenshiftSDNControllerLogs(t *testing.T) {
	t.Run("No log line matches the messages to search", func(t *testing.T) {
		// Given
		mock := "logline"
		test := getSDNControllerLogsMessagesFilter()
		logline := bufio.NewScanner(strings.NewReader(mock))

		// When
		result, err := common.FilterLogFromScanner(logline, test.MessagesToSearch, test.IsRegexSearch, nil)

		// Assert
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("'Node is not Ready' search matches successfully", func(t *testing.T) {
		// Given
		mock := "Node 'test' is not Ready"
		test := getSDNControllerLogsMessagesFilter()
		logline := bufio.NewScanner(strings.NewReader(mock))

		// When
		result, err := common.FilterLogFromScanner(logline, test.MessagesToSearch, test.IsRegexSearch, nil)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, mock, result)
	})

	t.Run("'Node may be offline' search matches successfully", func(t *testing.T) {
		// Given
		mock := "Node 'test' may be offline... retrying"
		test := getSDNControllerLogsMessagesFilter()
		logline := bufio.NewScanner(strings.NewReader(mock))

		// When
		result, err := common.FilterLogFromScanner(logline, test.MessagesToSearch, test.IsRegexSearch, nil)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, mock, result)
	})

	t.Run("'Node is offline' search matches successfully", func(t *testing.T) {
		// Given
		mock := "Node 'test' is offline"
		test := getSDNControllerLogsMessagesFilter()
		logline := bufio.NewScanner(strings.NewReader(mock))

		// When
		result, err := common.FilterLogFromScanner(logline, test.MessagesToSearch, test.IsRegexSearch, nil)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, mock, result)
	})

	t.Run("'Node is back online' search matches successfully", func(t *testing.T) {
		// Given
		mock := "Node 'test' is back online"
		test := getSDNControllerLogsMessagesFilter()
		logline := bufio.NewScanner(strings.NewReader(mock))

		// When
		result, err := common.FilterLogFromScanner(logline, test.MessagesToSearch, test.IsRegexSearch, nil)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, mock, result)
	})
}
