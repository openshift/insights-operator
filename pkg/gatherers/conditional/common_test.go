package conditional

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// nolint:dupl
func Test_getAlertPodName(t *testing.T) {
	tests := []struct {
		name    string
		labels  AlertLabels
		want    string
		wantErr bool
	}{
		{
			name:    "Pod name exists",
			labels:  AlertLabels{"pod": "test-name"},
			want:    "test-name",
			wantErr: false,
		},
		{
			name:    "Pod name does not exists",
			labels:  AlertLabels{},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAlertPodName(tt.labels)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

// nolint:dupl
func Test_getAlertPodNamespace(t *testing.T) {
	tests := []struct {
		name    string
		labels  AlertLabels
		want    string
		wantErr bool
	}{
		{
			name:    "Pod namemespace exists",
			labels:  AlertLabels{"namespace": "test-namespace"},
			want:    "test-namespace",
			wantErr: false,
		},
		{
			name:    "Pod namespace does not exists",
			labels:  AlertLabels{},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAlertPodNamespace(tt.labels)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

// nolint:dupl
func TestGetAlertPodContainer(t *testing.T) {
	testCases := []struct {
		name           string
		labels         AlertLabels
		expectedResult string
		expectedErr    error
	}{
		{
			name: "valid container label",
			labels: AlertLabels{
				"container": "my-container",
			},
			expectedResult: "my-container",
			expectedErr:    nil,
		},
		{
			name:           "missing container label",
			labels:         AlertLabels{},
			expectedResult: "",
			expectedErr:    ErrAlertPodContainerMissing,
		},
		{
			name: "empty container label",
			labels: AlertLabels{
				"container": "",
			},
			expectedResult: "",
			expectedErr:    ErrAlertPodContainerMissing,
		},
		{
			name:           "nil labels",
			labels:         nil,
			expectedResult: "",
			expectedErr:    ErrAlertPodContainerMissing,
		},
		{
			name: "valid labels but missing container key",
			labels: AlertLabels{
				"namespace": "my-namespace",
			},
			expectedResult: "",
			expectedErr:    ErrAlertPodContainerMissing,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			container, err := getAlertPodContainer(tc.labels)
			assert.Equal(t, tc.expectedResult, container)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}
