package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func createMockCostManagementMetricsConfig(t *testing.T, c dynamic.Interface, data string) {
	decUnstructured1 := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testCostManagementMetricsConfig := &unstructured.Unstructured{}
	_, _, err := decUnstructured1.Decode([]byte(data), nil, testCostManagementMetricsConfig)
	if err != nil {
		t.Fatal("unable to decode CostManagementMetricsConfig YAML", err)
	}

	_, _ = c.
		Resource(costManagementMetricsConfigResource).
		Create(context.Background(), testCostManagementMetricsConfig, metav1.CreateOptions{})
}

func Test_CostManagementMetricsConfigs(t *testing.T) {
	// Initialize the fake dynamic client.
	costMgmtMetricsConfigClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			costManagementMetricsConfigResource: "CostManagementMetricsConfigList",
		})

	records, errs := gatherCostManagementMetricsConfigs(context.Background(), costMgmtMetricsConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 0 records because there is no CostManagementMetricsConfigs yet.
	if len(records) != 0 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}

	// Create first CostManagementMetricsConfig resource.
	costMgmtMetricsConfigYAML1 := `apiVersion: costmanagement-metrics-cfg.openshift.io/v1beta1
kind: CostManagementMetricsConfig
metadata:
    name: costmanagementmetricscfg-sample-1
`

	createMockCostManagementMetricsConfig(t, costMgmtMetricsConfigClient, costMgmtMetricsConfigYAML1)
	records, errs = gatherCostManagementMetricsConfigs(context.Background(), costMgmtMetricsConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 1 record because there is now 1 CostManagementMetricsConfig resource.
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}

	// Create second CostManagementMetricsConfig resource.
	costMgmtMetricsConfigYAML2 := `apiVersion: costmanagement-metrics-cfg.openshift.io/v1beta1
kind: CostManagementMetricsConfig
metadata:
    name: costmanagementmetricscfg-sample-2
`

	createMockCostManagementMetricsConfig(t, costMgmtMetricsConfigClient, costMgmtMetricsConfigYAML2)
	records, errs = gatherCostManagementMetricsConfigs(context.Background(), costMgmtMetricsConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 2 record because there are now 2 CostManagementMetricsConfig resource.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the third run: %d", len(records))
	}

	// Create third CostManagementMetricsConfig resource.
	costMgmtMetricsConfigYAML3 := `apiVersion: costmanagement-metrics-cfg.openshift.io/v1beta1
kind: CostManagementMetricsConfig
metadata:
    name: costmanagementmetricscfg-sample-3
    spec:
       authentication:
            type: basic
            secret_name: console_basic_auth
`

	createMockCostManagementMetricsConfig(t, costMgmtMetricsConfigClient, costMgmtMetricsConfigYAML3)
	records, errs = gatherCostManagementMetricsConfigs(context.Background(), costMgmtMetricsConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 3 record because there are now 3 CostManagementMetricsConfig resource.
	if len(records) != 3 {
		t.Fatalf("unexpected number or records in the fourth run: %d", len(records))
	}

	// Create fourth CostManagementMetricsConfig resource.
	costMgmtMetricsConfigYAML4 := `apiVersion: costmanagement-metrics-cfg.openshift.io/v1beta1
kind: CostManagementMetricsConfig
metadata:
    name: costmanagementmetricscfg-sample-4
    spec:
        authentication:
            type: token
`

	createMockCostManagementMetricsConfig(t, costMgmtMetricsConfigClient, costMgmtMetricsConfigYAML4)
	records, errs = gatherCostManagementMetricsConfigs(context.Background(), costMgmtMetricsConfigClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 4 record because there are now 4 CostManagementMetricsConfig resource.
	if len(records) != 4 {
		t.Fatalf("unexpected number or records in the fifth run: %d", len(records))
	}
}
