package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sumErrors(t *testing.T) {
	tests := []struct {
		name           string
		inputErr       []error
		expectNil      bool
		expectedErrStr string
	}{
		{
			name:           "empty slice of errors",
			inputErr:       []error{},
			expectNil:      true,
			expectedErrStr: "",
		},
		{
			name:           "single error as input",
			inputErr:       []error{fmt.Errorf("test error")},
			expectedErrStr: "test error",
		},
		{
			name: "multiple errors as input",
			inputErr: []error{
				fmt.Errorf("error 3"),
				fmt.Errorf("error 3"),
				fmt.Errorf("error 2"),
				fmt.Errorf("error 1"),
				fmt.Errorf("error 5"),
			},
			expectedErrStr: "error 3, error 2, error 1, error 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errResult := UniqueErrors(tt.inputErr)
			if tt.expectNil {
				assert.NoError(t, errResult)
			} else {
				assert.EqualError(t, errResult, tt.expectedErrStr)
			}
		})
	}
}
