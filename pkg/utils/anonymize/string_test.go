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
