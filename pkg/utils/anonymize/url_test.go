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
			name:     "URL with port",
			input:    "https://example.com:8080/path",
			expected: "xxxxx://xxxxxxx.xxx:xxxx/xxxx",
		},
		{
			name:     "URL with subdomain",
			input:    "https://api.example.com/v1/users",
			expected: "xxxxx://xxx.xxxxxxx.xxx/xx/xxxxx",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "URL with fragment",
			input:    "https://example.com/page#section",
			expected: "xxxxx://xxxxxxx.xxx/xxxxxxxxxxxx",
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
			name: "single URL",
			input: []string{
				"https://example.com/path",
			},
			expected: []string{
				"xxxxx://xxxxxxx.xxx/xxxx",
			},
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
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
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
			name:     "single URL",
			input:    "https://example.com/path",
			expected: "xxxxx://xxxxxxx.xxx/xxxx",
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
		{
			name:     "URLs with query parameters",
			input:    "https://example.com/path?a=1,https://example.com/path?b=2",
			expected: "xxxxx://xxxxxxx.xxx/xxxxxxxx,xxxxx://xxxxxxx.xxx/xxxxxxxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := URLCSV(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
