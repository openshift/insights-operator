package clusterconfig

import (
	"testing"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/marshal"
	"github.com/stretchr/testify/assert"
)

func Test_formatKubeVirtRecords(t *testing.T) {
	tests := []struct {
		name           string
		inputRecords   []record.Record
		expectedResult string
	}{
		{
			name:           "single record with timestamp and JSON",
			inputRecords:   []record.Record{{Item: marshal.Raw{Str: "2025-08-01T14:34:55.250444623Z {\"component\":\"virt-launcher\"}"}}},
			expectedResult: "{\"component\":\"virt-launcher\"}",
		},
		{
			name:           "record with no JSON content does not cause an error",
			inputRecords:   []record.Record{{Item: marshal.Raw{Str: "2025-08-01T14:34:55.250444623Z plain log message"}}},
			expectedResult: "",
		},
		{
			name: "record with nested JSON",
			inputRecords: []record.Record{{Item: marshal.Raw{
				Str: "2025-08-01T14:34:55.250444623Z {\"data\":{\"nested\":\"value\"},\"array\":[1,2,3]}"}},
			},
			expectedResult: "{\"data\":{\"nested\":\"value\"},\"array\":[1,2,3]}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			test, err := formatKubeVirtRecords(tt.inputRecords)

			// Assert
			assert.NoError(t, err)
			assert.Len(t, test, 1)

			content, isMarshalRaw := test[0].Item.(marshal.Raw)
			assert.True(t, isMarshalRaw)
			assert.Equal(t, tt.expectedResult, content.Str)
		})
	}
}
