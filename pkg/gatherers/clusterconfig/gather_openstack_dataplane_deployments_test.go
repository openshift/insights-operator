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

func Test_OpenStackDataPlaneDeployments_Gather(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		osdpdYAML  []string
		exp        []string
	}{
		{
			name:       "Test OpenStackDataPlaneDeployment CR exists",
			namespaces: []string{"openstack"},
			osdpdYAML:  []string{},
			exp:        []string{},
		},
		{
			name:       "Test single OpenStackDataPlaneDeployment CR",
			namespaces: []string{"openstack"},
			osdpdYAML: []string{`
apiVersion: dataplane.openstack.org/v1beta1
kind: OpenStackDataPlaneDeployment
metadata:
  name: test-cr
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
`},
			exp: []string{"namespaces/openstack/dataplane.openstack.org/openstackdataplanedeployments/test-cr"},
		},
		{
			name:       "Test Multiple OpenStackDataPlaneDeployment CRs",
			namespaces: []string{"openstack", "openstack"},
			osdpdYAML: []string{`
apiVersion: dataplane.openstack.org/v1beta1
kind: OpenStackDataPlaneDeployment
metadata:
  name: test-cr-1
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
`, `
apiVersion: dataplane.openstack.org/v1beta1
kind: OpenStackDataPlaneDeployment
metadata:
  name: test-cr-2
  namespace: openstack
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: test_configuration
`},
			exp: []string{
				"namespaces/openstack/dataplane.openstack.org/openstackdataplanedeployments/test-cr-1",
				"namespaces/openstack/dataplane.openstack.org/openstackdataplanedeployments/test-cr-2",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				osdpdGroupVersionResource: "osdpdList",
			})
			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
			testOsdpd := &unstructured.Unstructured{}
			ctx := context.Background()

			for i := range test.osdpdYAML {
				_, _, err := decUnstructured.Decode([]byte(test.osdpdYAML[i]), nil, testOsdpd)
				assert.NoError(t, err)
				_, err = dynamicClient.Resource(osdpdGroupVersionResource).
					Namespace(test.namespaces[i]).
					Create(ctx, testOsdpd, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			records, errs := gatherOpenstackDataplaneDeployments(ctx, dynamicClient)
			assert.Emptyf(t, errs, "Unexpected errors: %#v", errs)
			var recordNames []string
			for i := range records {
				recordNames = append(recordNames, records[i].Name)
				if len(test.osdpdYAML) > 0 {
					marshaledItem, _ := records[i].Item.Marshal()
					gatheredItem := unstructured.Unstructured{}
					err := json.Unmarshal(marshaledItem, &gatheredItem)
					assert.NoError(t, err)
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
