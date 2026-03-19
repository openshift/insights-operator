package utils

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_CountLines(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
		expectedErr   error
	}{
		{
			name:          "empty input",
			input:         "",
			expectedCount: 1,
			expectedErr:   io.EOF,
		},
		{
			name:          "single line with newline",
			input:         "single line\n",
			expectedCount: 2,
			expectedErr:   io.EOF,
		},
		{
			name:          "multiple lines",
			input:         "line1\nline2\nline3\n",
			expectedCount: 4,
			expectedErr:   io.EOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			count, err := CountLines(reader)
			assert.Equal(t, tt.expectedCount, count)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}
