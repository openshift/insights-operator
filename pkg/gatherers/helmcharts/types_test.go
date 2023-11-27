package helmcharts

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
