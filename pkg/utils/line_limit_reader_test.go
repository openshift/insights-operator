package utils

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_LineLimitReader_Read(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		lineLimit          int
		expectedOutput     string
		expectedTotalLines int
		expectEOF          bool
	}{
		{
			name:               "read exactly at line limit",
			input:              "line1\nline2\nline3\n",
			lineLimit:          3,
			expectedOutput:     "line1\nline2\nline3\n",
			expectedTotalLines: 3,
			expectEOF:          false,
		},
		{
			name:               "read beyond line limit",
			input:              "line1\nline2\nline3\nline4\nline5\n",
			lineLimit:          2,
			expectedOutput:     "line1\nline2\n",
			expectedTotalLines: 5,
			expectEOF:          true,
		},
		{
			name:               "zero line limit",
			input:              "line1\nline2\n",
			lineLimit:          0,
			expectedOutput:     "",
			expectedTotalLines: 0,
			expectEOF:          true,
		},
		{
			name:               "input without newlines",
			input:              "single line",
			lineLimit:          5,
			expectedOutput:     "single line",
			expectedTotalLines: 0,
			expectEOF:          false,
		},
		{
			name:               "empty input",
			input:              "",
			lineLimit:          5,
			expectedOutput:     "",
			expectedTotalLines: 0,
			expectEOF:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			limitedReader := NewLineLimitReader(reader, tt.lineLimit)

			n, err := io.ReadAll(limitedReader)

			if tt.expectEOF {
				// For limited reads, we expect some data followed by EOF
				assert.Equal(t, tt.expectedOutput, string(n))
			} else {
				// For unlimited or empty reads
				if err != nil && err != io.EOF {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(tt.input) > 0 {
					assert.Equal(t, tt.expectedOutput, string(n))
				}
			}

			assert.Equal(t, tt.expectedTotalLines, limitedReader.GetTotalLinesRead())
		})
	}
}
