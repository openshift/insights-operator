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

func Test_OpenStackVersions_Gather(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		osvYAML    []string
		exp        []string
	}{
		{
			name:       "Test OpenStacVersions CR not exists",
			namespaces: []string{"openstack"},
			osvYAML:    []string{},
			exp:        []string{},
		},
		{
			name:       "Test single OpenStacVersions CR",
			namespaces: []string{"openstack"},
			osvYAML: []string{`
apiVersion: core.openstack.org/v1beta1
kind: OpenStackVersion
metadata:
  name: test-cr
  namespace: openstack
`},
			exp: []string{"namespaces/openstack/core.openstack.org/openstackversions/test-cr"},
		},
		{
			name:       "Test Multiple OpenStackVersions CR",
			namespaces: []string{"openstack-1", "openstack-2"},
			osvYAML: []string{`
apiVersion: core.openstack.org/v1beta1
kind: OpenStackVersion
metadata:
  name: test-cr-1
  namespace: openstack-1
`, `
apiVersion: core.openstack.org/v1beta1
kind: OpenStackVersion
metadata:
  name: test-cr-2
  namespace: openstack-2
`},
			exp: []string{
				"namespaces/openstack-1/core.openstack.org/openstackversions/test-cr-1",
				"namespaces/openstack-2/core.openstack.org/openstackversions/test-cr-2",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				osvGroupVersionResource: "openstackversionsList",
			})
			decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
			testOsv := &unstructured.Unstructured{}
			ctx := context.Background()

			for i := range test.osvYAML {
				_, _, err := decUnstructured.Decode([]byte(test.osvYAML[i]), nil, testOsv)
				assert.NoError(t, err)
				_, err = dynamicClient.Resource(osvGroupVersionResource).
					Namespace(test.namespaces[i]).
					Create(ctx, testOsv, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			records, errs := GatherOpenstackVersions(ctx, dynamicClient)
			assert.Emptyf(t, errs, "Unexpected errors: %#v", errs)
			var recordNames []string
			for i := range records {
				recordNames = append(recordNames, records[i].Name)
			}
			assert.ElementsMatch(t, test.exp, recordNames)
		})
	}
}
