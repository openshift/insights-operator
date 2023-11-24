package workloads

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddItem(t *testing.T) {
	tests := []struct {
		name         string
		info         map[string]map[string][]HelmChartInfo
		expectedList map[string][]HelmChartInfo
	}{
		{
			name: "two namespaces, two charts and multiple resources",
			info: map[string]map[string][]HelmChartInfo{
				"mynamespace": {
					"deployments": {
						{Name: "mychart1", Version: "1.0.0"},
					},
					"statefulsets": {
						{Name: "mychart2", Version: "2.0.0"},
					},
				},
				"mynamespace2": {
					"deployments": {
						{Name: "mychart1", Version: "1.0.0"},
						{Name: "mychart1", Version: "1.0.0"},
					},
					"statefulsets": {
						{Name: "mychart1", Version: "1.0.0"},
						{Name: "mychart1", Version: "2.0.0"},
					},
				},
			},
			expectedList: map[string][]HelmChartInfo{
				"mynamespace": {
					{
						Name:      "mychart1",
						Version:   "1.0.0",
						Resources: map[string]int{"deployments": 1},
					},
					{
						Name:      "mychart2",
						Version:   "2.0.0",
						Resources: map[string]int{"statefulsets": 1},
					},
				},
				"mynamespace2": {
					{
						Name:      "mychart1",
						Version:   "1.0.0",
						Resources: map[string]int{"deployments": 2, "statefulsets": 1},
					},
					{
						Name:      "mychart1",
						Version:   "2.0.0",
						Resources: map[string]int{"statefulsets": 1},
					},
				},
			},
		},
		{
			name: "one namespace, two resources for the same chart",
			info: map[string]map[string][]HelmChartInfo{
				"mynamespace": {
					"deployments": {
						{Name: "mychart1", Version: "1.0.0"},
					},
					"statefulsets": {
						{Name: "mychart1", Version: "1.0.0"},
					},
				},
			},
			expectedList: map[string][]HelmChartInfo{
				"mynamespace": {
					{
						Name:      "mychart1",
						Version:   "1.0.0",
						Resources: map[string]int{"deployments": 1, "statefulsets": 1},
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helmList := newHelmChartInfoList()
			for namespace, resources := range tt.info {
				for resource, charts := range resources {
					for _, chartInfo := range charts {
						helmList.addItem(namespace, resource, chartInfo)
					}
				}
			}

			assert.Equal(t, tt.expectedList, helmList.Namespaces, "expected '%v', got '%v'", tt.expectedList, helmList.Namespaces)
		})
	}
}

func TestHelmChartNameAndVersion(t *testing.T) {
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
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotName, gotVersion := helmChartNameAndVersion(tt.args.chart)
			assert.Equalf(t, tt.wantName, gotName, "expected name to be '%s', got '%s'", tt.wantName, gotName)
			assert.Equalf(t, tt.wantVersion, gotVersion, "expected version to be '%s', got '%s'", tt.wantVersion, gotVersion)
		})
	}
}

func TestIsStringVersion(t *testing.T) {
	tests := []struct {
		version string
		isValid bool
	}{
		{"latest", true},
		{"beta", true},
		{"alpha", true},
		{"v1.2.3", false},
		{"1.2.3", false},
		{"", false},
	}

	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()

			result := isStringVersion(tt.version)
			assert.Equalf(t, tt.isValid, result, "Version '%s' expects to be '%v', got '%v'", tt.version, tt.isValid, result)
		})
	}
}
