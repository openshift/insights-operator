package anonymize

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_UnstructuredNestedStringField(t *testing.T) {
	tests := []struct {
		name         string
		data         map[string]interface{}
		fields       []string
		expectedData map[string]interface{}
		expectedErr  error
	}{
		{
			name: "anonymize top-level field",
			data: map[string]interface{}{
				"password": "secret123",
			},
			fields: []string{"password"},
			expectedData: map[string]interface{}{
				"password": "xxxxxxxxx",
			},
			expectedErr: nil,
		},
		{
			name: "anonymize nested field",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "secret123",
					"name":     "john",
				},
			},
			fields: []string{"user", "password"},
			expectedData: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "xxxxxxxxx",
					"name":     "john",
				},
			},
			expectedErr: nil,
		},
		{
			name: "anonymize deeply nested field",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"secret": "mysecret",
						},
					},
				},
			},
			fields: []string{"level1", "level2", "level3", "secret"},
			expectedData: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"secret": "xxxxxxxx",
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			name: "field not found",
			data: map[string]interface{}{
				"username": "john",
			},
			fields:       []string{"password"},
			expectedData: map[string]interface{}{"username": "john"},
			expectedErr:  fmt.Errorf("unable to find field '[password]'"),
		},
		{
			name: "nested field not found",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "john",
				},
			},
			fields:       []string{"user", "password"},
			expectedData: map[string]interface{}{"user": map[string]interface{}{"name": "john"}},
			expectedErr:  fmt.Errorf("unable to find field '[user password]'"),
		},
		{
			name: "anonymize empty string",
			data: map[string]interface{}{
				"field": "",
			},
			fields: []string{"field"},
			expectedData: map[string]interface{}{
				"field": "",
			},
			expectedErr: nil,
		},
		{
			name: "anonymize field with special characters",
			data: map[string]interface{}{
				"url": "https://example.com/path?query=value",
			},
			fields: []string{"url"},
			expectedData: map[string]interface{}{
				"url": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UnstructuredNestedStringField(tt.data, tt.fields...)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedData, tt.data)
			}
		})
	}
}
