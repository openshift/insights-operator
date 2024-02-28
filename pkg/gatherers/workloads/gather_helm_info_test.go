package workloads

import (
	"context"
	"testing"

	"github.com/openshift/insights-operator/pkg/record"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	"github.com/stretchr/testify/assert"
)

func TestGatherHelmInfo(t *testing.T) { // nolint: funlen
	ctx := context.TODO()

	hash, err := createHash("mynamespace")
	assert.NoError(t, err, "failed to generate namespace hash")

	// create the data for testing here
	helmChartInfoList := newHelmChartInfoList()
	helmChartInfoList.Namespaces[hash] = []HelmChartInfo{
		{
			Name:    "postgres",
			Version: "9.0.0",
			Resources: map[string]int{
				"daemonsets":   1,
				"deployments":  1,
				"replicasets":  1,
				"services":     1,
				"statefulsets": 1,
			},
		},
	}

	tests := []struct {
		name           string
		fakeClientFunc func() dynamic.Interface
		wantRecords    []record.Record
		wantErrors     int
	}{
		{
			name: "valid helm resources",
			fakeClientFunc: func() dynamic.Interface {
				fakeClient := fake.NewSimpleDynamicClient(runtime.NewScheme(), []runtime.Object{
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "mydeployment",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"app.kubernetes.io/managed-by": "Helm",
									"helm.sh/chart":                "postgres-9.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Replicaset",
							"metadata": map[string]interface{}{
								"name":      "myreplicaset",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"app.kubernetes.io/managed-by": "Helm",
									"helm.sh/chart":                "postgres-9.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Daemonset",
							"metadata": map[string]interface{}{
								"name":      "mydemonset",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"app.kubernetes.io/managed-by": "Helm",
									"helm.sh/chart":                "postgres-9.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Statefulset",
							"metadata": map[string]interface{}{
								"name":      "mystateful",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"app.kubernetes.io/managed-by": "Helm",
									"helm.sh/chart":                "postgres-9.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Service",
							"metadata": map[string]interface{}{
								"name":      "myservice",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"app.kubernetes.io/managed-by": "Helm",
									"helm.sh/chart":                "postgres-9.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
				}...)
				return fakeClient
			},
			wantRecords: []record.Record{
				{
					Name: "config/helmchart_info",
					Item: record.JSONMarshaller{Object: &helmChartInfoList.Namespaces},
				},
			},
			wantErrors: 0,
		},
		{
			name: "invalid helm resources",
			fakeClientFunc: func() dynamic.Interface {
				fakeClient := fake.NewSimpleDynamicClient(runtime.NewScheme(), []runtime.Object{
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "mydeployment",
								"namespace": "mynamespace",
								"labels":    map[string]interface{}{},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Replicaset",
							"metadata": map[string]interface{}{
								"name":      "myreplicaset",
								"namespace": "mynamespace",
								"labels":    nil,
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Daemonset",
							"metadata": map[string]interface{}{
								"name":      "mydemonset",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"app.kubernetes.io/managed-by": "Helm",
									"app.kubernetes.io/version":    "1.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Statefulset",
							"metadata": map[string]interface{}{
								"name":      "mystateful",
								"namespace": "mynamespace",
								"labels": map[string]interface{}{
									"helm.sh/chart": "postgres-9.0.0",
								},
							},
							"spec": map[string]interface{}{},
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Service",
							"metadata": map[string]interface{}{
								"name":      "myservice",
								"namespace": "mynamespace",
								"labels":    map[string]interface{}{},
							},
							"spec": map[string]interface{}{},
						},
					},
				}...)
				return fakeClient
			},
			wantRecords: nil,
			wantErrors:  0,
		},
	}

	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dynamicClient := tt.fakeClientFunc()
			records, errs := gatherHelmInfo(ctx, dynamicClient)

			assert.Equal(t, tt.wantRecords, records)
			assert.Len(t, errs, tt.wantErrors)
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
