package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const (
	controlPlaneMachineSetYAML = `
apiVersion: machine.openshift.io/v1
kind: ControlPlaneMachineSet
metadata:
    name: cluster
`

	controlPlaneMachineSet2YAML = `
apiVersion: machine.openshift.io/v1
kind: ControlPlaneMachineSet
metadata:
    name: cluster-2
    namespace: openshift-machine-api
`
)

func Test_GatherControlPlaneMachineSet(t *testing.T) {
	tests := []struct {
		name                string
		objects             []string
		namespaces          []string
		expectedRecordCount int
		expectedRecordNames []string
	}{
		{
			name:                "single resource with namespace",
			objects:             []string{controlPlaneMachineSetYAML},
			namespaces:          []string{"openshift-machine-api"},
			expectedRecordCount: 1,
			expectedRecordNames: []string{"config/controlplanemachinesets/openshift-machine-api/cluster"},
		},
		{
			name:                "single resource without namespace",
			objects:             []string{controlPlaneMachineSetYAML},
			namespaces:          []string{""},
			expectedRecordCount: 1,
			expectedRecordNames: []string{"config/controlplanemachinesets/cluster"},
		},
		{
			name:                "empty list",
			objects:             []string{},
			namespaces:          []string{},
			expectedRecordCount: 0,
			expectedRecordNames: []string{},
		},
		{
			name:                "multiple resources",
			objects:             []string{controlPlaneMachineSetYAML, controlPlaneMachineSet2YAML},
			namespaces:          []string{"openshift-machine-api", "openshift-machine-api"},
			expectedRecordCount: 2,
			expectedRecordNames: []string{
				"config/controlplanemachinesets/openshift-machine-api/cluster",
				"config/controlplanemachinesets/openshift-machine-api/cluster-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				controlPlaneMachineSetVersionResource: "ControlPlaneMachineSetList",
			})
			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

			for i, objYAML := range tt.objects {
				obj := &unstructured.Unstructured{}
				_, _, err := decUnstructured.Decode([]byte(objYAML), nil, obj)
				if err != nil {
					t.Fatalf("unable to decode controlplanemachineset %d: %v", i, err)
				}

				if tt.namespaces[i] != "" {
					_, err = client.Resource(controlPlaneMachineSetVersionResource).Namespace(tt.namespaces[i]).Create(
						context.Background(),
						obj,
						metav1.CreateOptions{},
					)
				} else {
					_, err = client.Resource(controlPlaneMachineSetVersionResource).Create(
						context.Background(),
						obj,
						metav1.CreateOptions{},
					)
				}
				if err != nil {
					t.Fatalf("unable to create fake controlplanemachineset %d: %v", i, err)
				}
			}

			ctx := context.Background()
			records, errs := gatherControlPlaneMachineSet(ctx, client)
			assert.Empty(t, errs)
			assert.Len(t, records, tt.expectedRecordCount)

			recordNames := make(map[string]bool)
			for _, record := range records {
				recordNames[record.Name] = true
			}
			for _, expectedName := range tt.expectedRecordNames {
				assert.True(t, recordNames[expectedName], "missing expected record: %s", expectedName)
			}
		})
	}
}
