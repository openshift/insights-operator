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

func Test_OpenStackControlPlanes_Gather(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		oscpYAML   []string
		exp        []string
	}{
		{
			name:       "Test OpenStackControlPlane CR exists",
			namespaces: []string{"openstack"},
			oscpYAML:   []string{},
			exp:        []string{},
		},
		{
			name:       "Test single OpenStackControlPlane CR",
			namespaces: []string{"openstack"},
			oscpYAML: []string{`
apiVersion: core.openstack.org/v1beta1
kind: OpenStackControlPlane
metadata:
  name: test-cr
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
spec:
  customService:
    string_field: test-string
`},
			exp: []string{"namespaces/openstack/core.openstack.org/openstackcontrolplanes/test-cr"},
		},
		{
			name:       "Test Multiple OpenStackControlPlane CR",
			namespaces: []string{"openstack-1", "openstack-2"},
			oscpYAML: []string{`
apiVersion: core.openstack.org/v1beta1
kind: OpenStackControlPlane
metadata:
  name: test-cr-1
  namespace: openstack-1
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
spec:
  customService:
    string_field: test-string
`, `
apiVersion: core.openstack.org/v1beta1
kind: OpenStackControlPlane
metadata:
  name: test-cr-2
  namespace: openstack-2
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
spec:
  customService:
    string_field: test-string
`},
			exp: []string{
				"namespaces/openstack-1/core.openstack.org/openstackcontrolplanes/test-cr-1",
				"namespaces/openstack-2/core.openstack.org/openstackcontrolplanes/test-cr-2",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				oscpGroupVersionResource: "openstackcontrolplanesList",
			})
			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
			testOscp := &unstructured.Unstructured{}
			ctx := context.Background()

			for i := range test.oscpYAML {
				_, _, err := decUnstructured.Decode([]byte(test.oscpYAML[i]), nil, testOscp)
				assert.NoError(t, err)
				_, err = dynamicClient.Resource(oscpGroupVersionResource).
					Namespace(test.namespaces[i]).
					Create(ctx, testOscp, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			records, errs := gatherOpenstackControlplanes(ctx, dynamicClient)
			assert.Emptyf(t, errs, "Unexpected errors: %#v", errs)
			var recordNames []string
			for i := range records {
				recordNames = append(recordNames, records[i].Name)
				if len(test.oscpYAML) > 0 {
					marshaledItem, _ := records[i].Item.Marshal()
					gatheredItem := unstructured.Unstructured{}
					err := json.Unmarshal(marshaledItem, &gatheredItem)
					assert.Emptyf(t, err, "Error while unmarhaling json item")
					stringFieldValue, found, err := unstructured.NestedFieldCopy(gatheredItem.Object, "spec", "customService", "string_field")
					assert.True(t, found, "Field 'string_field' was not found in the gathered object")
					assert.Emptyf(t, err, "Unexpected error: %#v", err)
					assert.Exactly(t, "test-string", stringFieldValue)
					_, lastAppliedConfigurationFound, _ := unstructured.NestedFieldCopy(
						gatheredItem.Object,
						"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")
					assert.Emptyf(t, lastAppliedConfigurationFound,
						"Field 'metadata/annotations/kubectl.kubernetes.io/last-applied-configuration' was not removed from the gathered object")
				}
			}
			assert.ElementsMatch(t, test.exp, recordNames)
		})
	}
}
