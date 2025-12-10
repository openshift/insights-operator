package record

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockMarshaller is a test helper for testing different extensions
type mockMarshaller struct {
	extension string
	data      []byte
	err       error
}

func (m *mockMarshaller) Marshal() ([]byte, error) {
	return m.data, m.err
}

func (m *mockMarshaller) GetExtension() string {
	return m.extension
}

func Test_Record_Marshal(t *testing.T) {
	tests := []struct {
		name        string
		record      Record
		expectError bool
	}{
		{
			name: "marshal record with valid JSONMarshaller",
			record: Record{
				Name: "test-record",
				Item: JSONMarshaller{Object: map[string]string{"key": "value"}},
			},
			expectError: false,
		},
		{
			name: "marshal record with struct",
			record: Record{
				Name: "struct-record",
				Item: JSONMarshaller{Object: struct{ Name string }{Name: "test"}},
			},
			expectError: false,
		},
		{
			name: "marshal record with invalid content (channel) returns error",
			record: Record{
				Name: "invalid-record",
				Item: JSONMarshaller{Object: make(chan int)},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, fingerprint, err := tt.record.Marshal()

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, fingerprint)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, content)
				assert.NotEmpty(t, fingerprint, "Fingerprint should not be empty")
				assert.Len(t, fingerprint, 64, "SHA256 fingerprint should be 64 hex characters")

				// Verify fingerprint is valid SHA256 of content
				h := sha256.New()
				h.Write(content)
				expectedFp := hex.EncodeToString(h.Sum(nil))
				assert.Equal(t, expectedFp, fingerprint,
					"Fingerprint should be valid SHA256 of content")
			}
		})
	}
}

func Test_Record_GetFilename(t *testing.T) {
	tests := []struct {
		name             string
		record           Record
		expectedFilename string
	}{
		{
			name: "filename with json extension",
			record: Record{
				Name: "test-file",
				Item: JSONMarshaller{Object: "data"},
			},
			expectedFilename: "test-file.json",
		},
		{
			name: "filename with custom extension",
			record: Record{
				Name: "custom-file",
				Item: &mockMarshaller{extension: "yaml"},
			},
			expectedFilename: "custom-file.yaml",
		},
		{
			name: "filename with empty extension",
			record: Record{
				Name: "no-extension",
				Item: &mockMarshaller{extension: ""},
			},
			expectedFilename: "no-extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.record.GetFilename()
			assert.Equal(t, tt.expectedFilename, result)
		})
	}
}

func Test_JSONMarshaller_Marshal(t *testing.T) {
	tests := []struct {
		name        string
		marshaller  JSONMarshaller
		expectError bool
	}{
		{
			name:        "marshal simple struct",
			marshaller:  JSONMarshaller{Object: struct{ Name string }{Name: "test"}},
			expectError: false,
		},
		{
			name:        "marshal invalid type (channel)",
			marshaller:  JSONMarshaller{Object: make(chan int)},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.marshaller.Marshal()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Greater(t, len(result), 0)
			}
		})
	}
}

func Test_MemoryRecords_Sorting(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		name          string
		records       MemoryRecords
		expectedOrder []string
	}{
		{
			name: "sort records in descending time order",
			records: MemoryRecords{
				{Name: "oldest", At: baseTime.Add(-2 * time.Hour)},
				{Name: "newest", At: baseTime},
				{Name: "middle", At: baseTime.Add(-1 * time.Hour)},
			},
			expectedOrder: []string{"newest", "middle", "oldest"},
		},
		{
			name: "records with same timestamp maintain stable sort",
			records: MemoryRecords{
				{Name: "same-time-1", At: baseTime},
				{Name: "same-time-2", At: baseTime},
				{Name: "earlier", At: baseTime.Add(-1 * time.Hour)},
			},
			expectedOrder: []string{"same-time-1", "same-time-2", "earlier"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(tt.records)
			assert.Equal(t, len(tt.expectedOrder), len(tt.records))
			for i, expectedName := range tt.expectedOrder {
				assert.Equal(t, expectedName, tt.records[i].Name)
			}
		})
	}
}
