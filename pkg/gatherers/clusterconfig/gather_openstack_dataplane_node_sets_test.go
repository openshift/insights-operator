package clusterconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func assertFieldValue(t *testing.T, data map[string]interface{}, expectedValue string, fields ...string) {
	actualValue, found, err := unstructured.NestedFieldCopy(data, fields...)
	if !found {
		t.Fatalf("Field '%s' was not found in the gathered object", fields)
	}
	if err != nil {
		t.Fatal(err)
	}
	assert.Exactly(t, expectedValue, actualValue)
}

func Test_OpenStackDataPlaneNodeSets_Gather(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		osdpnsYAML []string
		exp        []string
	}{
		{
			name:       "Test OpenStackDataPlaneNodeSet CR exists",
			namespaces: []string{"openstack"},
			osdpnsYAML: []string{},
			exp:        []string{},
		},
		{
			name:       "Test single OpenStackDataPlaneNodeSet CR",
			namespaces: []string{"openstack"},
			osdpnsYAML: []string{`
apiVersion: dataplane.openstack.org/v1beta1
kind: OpenStackDataPlaneNodeSet
metadata:
  name: test-cr
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
spec:
  nodeTemplate:
    ansibleUser: openstack
  customService:
    string_field: test-string
status:
  allHostnames:
    edpm-compute-0:
      ctlplane: edpm-compute-0.ctlplane.example.com
      internalapi: edpm-compute-0.internalapi.example.com
`},
			exp: []string{"namespaces/openstack/dataplane.openstack.org/openstackdataplanenodesets/test-cr"},
		},
		{
			name:       "Test Multiple OpenStackDataPlaneNodeSet CRs",
			namespaces: []string{"openstack", "openstack"},
			osdpnsYAML: []string{`
apiVersion: dataplane.openstack.org/v1beta1
kind: OpenStackDataPlaneNodeSet
metadata:
  name: test-cr-1
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
spec:
  nodeTemplate:
    ansibleUser: openstack
  customService:
    string_field: test-string
status:
  allHostnames:
    edpm-compute-0:
      ctlplane: edpm-compute-0.ctlplane.example.com
      internalapi: edpm-compute-0.internalapi.example.com
`, `
apiVersion: dataplane.openstack.org/v1beta1
kind: OpenStackDataPlaneNodeSet
metadata:
  name: test-cr-2
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
spec:
  nodeTemplate:
    ansibleUser: openstack
  customService:
    string_field: test-string
status:
  allHostnames:
    edpm-compute-0:
      ctlplane: edpm-compute-0.ctlplane.example.com
      internalapi: edpm-compute-0.internalapi.example.com
`},
			exp: []string{
				"namespaces/openstack/dataplane.openstack.org/openstackdataplanenodesets/test-cr-1",
				"namespaces/openstack/dataplane.openstack.org/openstackdataplanenodesets/test-cr-2",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				osdpnsGroupVersionResource: "osdpnsList",
			})
			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
			testOsdpns := &unstructured.Unstructured{}
			ctx := context.Background()

			for i := range test.osdpnsYAML {
				_, _, err := decUnstructured.Decode([]byte(test.osdpnsYAML[i]), nil, testOsdpns)
				assert.NoError(t, err)
				_, err = dynamicClient.Resource(osdpnsGroupVersionResource).
					Namespace(test.namespaces[i]).
					Create(ctx, testOsdpns, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			records, errs := gatherOpenstackDataplaneNodeSets(ctx, dynamicClient)
			assert.Emptyf(t, errs, "Unexpected errors: %#v", errs)
			var recordNames []string
			for i := range records {
				recordNames = append(recordNames, records[i].Name)
				if len(test.osdpnsYAML) > 0 {
					marshaledItem, _ := records[i].Item.Marshal()
					gatheredItem := unstructured.Unstructured{}
					err := json.Unmarshal(marshaledItem, &gatheredItem)
					assert.NoError(t, err)
					assertFieldValue(
						t, gatheredItem.Object,
						"test-string",
						"spec", "customService", "string_field")
					assertFieldValue(
						t, gatheredItem.Object,
						"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						"status", "allHostnames", "edpm-compute-0", "ctlplane")
					assertFieldValue(
						t, gatheredItem.Object,
						"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						"status", "allHostnames", "edpm-compute-0", "internalapi")
					assertFieldValue(
						t, gatheredItem.Object,
						"xxxxxxxxx",
						"spec", "nodeTemplate", "ansibleUser")
					assertFieldValue(
						t, gatheredItem.Object,
						"xxxxxxxxx",
						"spec", "nodeTemplate", "ansibleUser")
					_, lastAppliedConfigurationFound, err := unstructured.NestedFieldCopy(
						gatheredItem.Object,
						"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")
					assert.NoError(t, err)
					assert.Emptyf(t, lastAppliedConfigurationFound,
						"Field 'metadata/annotations/kubectl.kubernetes.io/last-applied-configuration' was not removed from the gathered object")
				}
			}
			assert.ElementsMatch(t, test.exp, recordNames)
		})
	}
}
