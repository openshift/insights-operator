package anonymize

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_URL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic URL",
			input:    "https://example.com/path",
			expected: "xxxxx://xxxxxxx.xxx/xxxx",
		},
		{
			name:     "URL with query parameters",
			input:    "https://example.com/path?query=value",
			expected: "xxxxx://xxxxxxx.xxx/xxxxxxxxxxxxxxxx",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "IP address URL",
			input:    "http://192.168.1.1:8080/path",
			expected: "xxxx://xxx.xxx.x.x:xxxx/xxxx",
		},
		{
			name:     "URL with username and password",
			input:    "https://user:pass@example.com/path",
			expected: "xxxxx://xxxx:xxxxxxxxxxxx.xxx/xxxx",
		},
		{
			name:     "keeps dots, dashes, slashes, and colons",
			input:    "https://my-api.example-domain.co.uk:443/v1/resource/123",
			expected: "xxxxx://xx-xxx.xxxxxxx-xxxxxx.xx.xx:xxx/xx/xxxxxxxx/xxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := URL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_URLSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name: "multiple URLs",
			input: []string{
				"https://example.com/path1",
				"https://example.com/path2",
			},
			expected: []string{
				"xxxxx://xxxxxxx.xxx/xxxxx",
				"xxxxx://xxxxxxx.xxx/xxxxx",
			},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: nil,
		},
		{
			name: "URLs with different schemes",
			input: []string{
				"http://example.com",
				"https://example.com",
				"ftp://example.com",
			},
			expected: []string{
				"xxxx://xxxxxxx.xxx",
				"xxxxx://xxxxxxx.xxx",
				"xxx://xxxxxxx.xxx",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := URLSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_URLCSV(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple URLs separated by commas",
			input:    "https://example.com/path1,https://example.com/path2",
			expected: "xxxxx://xxxxxxx.xxx/xxxxx,xxxxx://xxxxxxx.xxx/xxxxx",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "URLs with spaces",
			input:    "https://example.com/path1, https://example.com/path2",
			expected: "xxxxx://xxxxxxx.xxx/xxxxx,xxxxxx://xxxxxxx.xxx/xxxxx",
		},
		{
			name:     "three URLs",
			input:    "http://api.example.com,https://web.example.com,https://mobile.example.com",
			expected: "xxxx://xxx.xxxxxxx.xxx,xxxxx://xxx.xxxxxxx.xxx,xxxxx://xxxxxx.xxxxxxx.xxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := URLCSV(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
