package clusterconfig

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func Test_Machine_Gather(t *testing.T) {
	tests := []struct {
		name        string
		machineYAML []string
		exp         []string
		expLen      int
	}{
		{
			name: "one machine",
			machineYAML: []string{`
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: test-master
`},
			exp:    []string{"config/machines/test-master"},
			expLen: 1,
		},
		{
			name:        "no machine",
			machineYAML: []string{},
			exp:         []string{},
			expLen:      0,
		},
		{
			name: "multiple machines",
			machineYAML: []string{`
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: machine-one
`, `
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: machine-two
`, `
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: machine-three
`, `
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: machine-four
`, `
apiversion: machine.openshift.io/v1beta1
kind: Machine
metadata:
    name: machine-five
`},
			exp: []string{"config/machines/machine-one",
				"config/machines/machine-two",
				"config/machines/machine-three",
				"config/machines/machine-four",
				"config/machines/machine-five"},
			expLen: 5,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machines"}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				gvr: "MachineList",
			})
			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

			testMachine := &unstructured.Unstructured{}

			for i := range test.machineYAML {
				_, _, err := decUnstructured.Decode([]byte(test.machineYAML[i]), nil, testMachine)
				if err != nil {
					t.Fatal("unable to decode machine ", err)
				}
				_, err = client.Resource(gvr).Create(context.Background(), testMachine, metav1.CreateOptions{})
				if err != nil {
					t.Fatal("unable to create fake machine ", err)
				}
			}

			ctx := context.Background()
			records, errs := gatherMachine(ctx, client)
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if len(records) != test.expLen {
				t.Fatalf("unexpected number of records %d", len(records))
			}
			tmp := []string{}
			for i := range records {
				tmp = append(tmp, records[i].Name)
			}
			sort.Strings(tmp)
			sort.Strings(test.exp)
			assert.Equal(t, test.exp, tmp)
		})
	}
}
