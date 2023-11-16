package workloads

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_helmChartNameAndVersion(t *testing.T) {
	type args struct {
		chart string
	}
	tests := []struct {
		name        string
		args        args
		wantName    string
		wantVersion string
	}{
		{
			name:        "Test with composed valid chart name and version",
			args:        args{chart: "nginx-server-1.2.3"},
			wantName:    "nginx-server",
			wantVersion: "1.2.3",
		},
		{
			name:        "Test with simple valid chart name and version",
			args:        args{chart: "postgres-2.1.0"},
			wantName:    "postgres",
			wantVersion: "2.1.0",
		},
		{
			name:        "Test with simple valid chart name but no version",
			args:        args{chart: "postgres"},
			wantName:    "postgres",
			wantVersion: "",
		},
		{
			name:        "Test with composed valid chart name but no version",
			args:        args{chart: "postgres-alpine"},
			wantName:    "postgres-alpine",
			wantVersion: "",
		},
		{
			name:        "Test with composed valid chart name and latest",
			args:        args{chart: "postgres-alpine-latest"},
			wantName:    "postgres-alpine",
			wantVersion: "latest",
		},
		{
			name:        "Test with 3 parts valid chart name no version",
			args:        args{chart: "postgres-alpine-chart"},
			wantName:    "postgres-alpine-chart",
			wantVersion: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := helmChartNameAndVersion(tt.args.chart)
			assert.Equalf(t, tt.wantName, got, "helmChartNameAndVersion(%v)", tt.args.chart)
			assert.Equalf(t, tt.wantVersion, got1, "helmChartNameAndVersion(%v)", tt.args.chart)
		})
	}
}
