package conditional

import "testing"

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
			if (err != nil) != tt.wantErr {
				t.Errorf("getAlertPodName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getAlertPodName() got = %v, want %v", got, tt.want)
			}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("getAlertPodNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getAlertPodNamespace() got = %v, want %v", got, tt.want)
			}
		})
	}
}
