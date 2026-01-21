package anonymize

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_String(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic string",
			input:    "hello",
			expected: "xxxxx",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			expected: "x",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "xxxxxxxxxxx",
		},
		{
			name:     "string with special characters",
			input:    "test@example.com",
			expected: "xxxxxxxxxxxxxxxx",
		},
		{
			name:     "long string",
			input:    "this is a very long string with many characters",
			expected: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, len(tt.input), len(result))
		})
	}
}

func Test_Bytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "basic byte slice",
			input:    []byte("hello"),
			expected: []byte("xxxxx"),
		},
		{
			name:     "empty byte slice",
			input:    []byte(""),
			expected: []byte(""),
		},
		{
			name:     "single byte",
			input:    []byte("a"),
			expected: []byte("x"),
		},
		{
			name:     "byte slice with spaces",
			input:    []byte("hello world"),
			expected: []byte("xxxxxxxxxxx"),
		},
		{
			name:     "byte slice with special characters",
			input:    []byte("test@example.com"),
			expected: []byte("xxxxxxxxxxxxxxxx"),
		},
		{
			name:     "nil byte slice",
			input:    nil,
			expected: []byte(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Bytes(tt.input)
			assert.Equal(t, tt.expected, result)
			if tt.input != nil {
				assert.Equal(t, len(tt.input), len(result))
			}
		})
	}
}
