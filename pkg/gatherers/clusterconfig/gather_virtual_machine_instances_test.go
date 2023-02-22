package clusterconfig

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/openshift/insights-operator/pkg/record"

	"github.com/stretchr/testify/assert"
)

func Test_gatherVirtualMachineInstances(t *testing.T) {
	var dataYAML = `
apiVersion: kubevirt.io/v1alpha3
kind: VirtualMachineInstance
metadata:
  name: testmachine
  namespace: default
spec:
  volumes:
  - cloudInitNoCloud:
      userData: |-
        #cloud-config
        user: cloud-user
        password: lymp-fda4-m1cv
        chpasswd: { expire: False }
    name: cloudinitdisk
`

	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testDataUnstructured := &unstructured.Unstructured{}

	_, _, err := decoder.Decode([]byte(dataYAML), nil, testDataUnstructured)
	assert.NoErrorf(t, err, "unable to decode virtualmachineinstances")

	tests := []struct {
		name             string
		gvrList          map[schema.GroupVersionResource]string
		dataUnstructured *unstructured.Unstructured
		want             []record.Record
		wantErrs         []error
	}{
		{
			name:             "Successfully collects VMI",
			gvrList:          map[schema.GroupVersionResource]string{virtualMachineInstancesResource: "VirtualMachineInstanceList"},
			dataUnstructured: testDataUnstructured,
			want: []record.Record{
				{
					Name: "config/virtualmachineinstances/default/testmachine",
					Item: record.ResourceMarshaller{Resource: anonymizeVirtualMachineInstances(testDataUnstructured)},
				},
			},
			wantErrs: nil,
		},
	}
	for _, test := range tests {
		tt := test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), tt.gvrList)
			_, err := dynamicClient.Resource(virtualMachineInstancesResource).
				Namespace("default").
				Create(ctx, tt.dataUnstructured, metav1.CreateOptions{})
			assert.NoErrorf(t, err, "unable to create fake virtualmachineinstances")

			got, gotErrs := gatherVirtualMachineInstances(ctx, dynamicClient)
			assert.Equalf(t, tt.want, got, "gatherVirtualMachineInstances(%v, %v)", ctx, dynamicClient)
			assert.Equalf(t, tt.wantErrs, gotErrs, "gatherVirtualMachineInstances(%v, %v)", ctx, dynamicClient)
		})
	}
}
