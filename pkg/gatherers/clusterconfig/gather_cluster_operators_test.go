package clusterconfig

import (
	"context"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	openshiftscheme "github.com/openshift/client-go/config/clientset/versioned/scheme"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var gvr = schema.GroupVersionResource{Group: "operator.openshift.io", Version: "v1", Resource: "testcontroller"}

func createTestClusterOperator(cli *configfake.Clientset) (*configv1.ClusterOperator, error) {
	co := &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusteroperator",
		},
		Spec: configv1.ClusterOperatorSpec{},
		Status: configv1.ClusterOperatorStatus{
			RelatedObjects: []configv1.ObjectReference{
				{
					Group:    "operator.openshift.io",
					Resource: "testcontroller",
					Name:     "foo",
				},
				{
					Group:    "",
					Resource: "anotherTestResource",
					Name:     "bar",
				},
			},
		}}

	return cli.ConfigV1().ClusterOperators().Create(context.Background(), co, metav1.CreateOptions{})
}

func createTestRelatedObject(dynamicCli *dynamicfake.FakeDynamicClient) (*unstructured.Unstructured, error) {
	var yamlDefinition = `
apiVersion: operator.openshift.io/v1
kind: TestController
metadata:
    name: foo
`
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	unstructuredObj := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(yamlDefinition), nil, unstructuredObj)
	if err != nil {
		return nil, err
	}
	return dynamicCli.Resource(gvr).Create(context.Background(), unstructuredObj, metav1.CreateOptions{})
}

func createFakeDynamicClient() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		gvr: "TestControllersList",
	})
}
func Test_Operators_GatherClusterOperators(t *testing.T) {
	cfg := configfake.NewSimpleClientset()
	_, err := createTestClusterOperator(cfg)
	assert.NoError(t, err, "unable to create fake clusteroperator")

	records, err := gatherClusterOperators(
		context.Background(),
		cfg.ConfigV1(),
		cfg.Discovery(),
		createFakeDynamicClient(),
	)
	if err != nil {
		t.Errorf("unexpected errors: %#v", err)
		return
	}

	item, _ := records[0].Item.Marshal()
	var gatheredCO configv1.ClusterOperator
	openshiftCodec := openshiftscheme.Codecs.LegacyCodec(configv1.GroupVersion)
	_, _, err = openshiftCodec.Decode(item, nil, &gatheredCO)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredCO.Name != "test-clusteroperator" {
		t.Fatalf("unexpected clusteroperator name %s", gatheredCO.Name)
	}
}

func Test_Operators_ClusterOperatorsRecords(t *testing.T) {
	type args struct {
		ctx             context.Context
		items           []configv1.ClusterOperator
		dynamicClient   dynamic.Interface
		discoveryClient discovery.DiscoveryInterface
	}
	tests := []struct {
		name string
		args args
		want []record.Record
	}{
		{
			name: "empty cluster operator",
			args: args{
				ctx:             context.TODO(),
				items:           []configv1.ClusterOperator{},
				dynamicClient:   &dynamicfake.FakeDynamicClient{},
				discoveryClient: kubefake.NewSimpleClientset().Discovery(),
			},
			want: []record.Record{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clusterOperatorsRecords(
				tt.args.ctx,
				tt.args.items,
				tt.args.dynamicClient,
				tt.args.discoveryClient); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clusterOperatorsRecords() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_Operators_collectClusterOperatorRelatedObjects(t *testing.T) {
	// create test clusteroperator resource
	co, err := createTestClusterOperator(configfake.NewSimpleClientset())
	assert.NoError(t, err, "unable to create fake clusteroperator")
	dynamicFake := createFakeDynamicClient()
	// create test related object to clusteroperator resource
	_, err = createTestRelatedObject(dynamicFake)
	assert.NoError(t, err, "unable to create fake related object")

	type args struct {
		ctx           context.Context
		dynamicClient dynamic.Interface
		co            configv1.ClusterOperator
		resVer        map[schema.GroupResource]string
	}
	tests := []struct {
		name string
		args args
		want []clusterOperatorResource
	}{
		{
			name: "cluster operator relatedObject obtained",
			args: args{
				ctx:           context.Background(),
				dynamicClient: dynamicFake,
				co:            *co,
				resVer: map[schema.GroupResource]string{
					{Group: "operator.openshift.io", Resource: "testcontroller"}: "v1",
				},
			},
			want: []clusterOperatorResource{
				{
					APIVersion: "operator.openshift.io/v1",
					Kind:       "TestController",
					Name:       "foo",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualClusterOperatorRelObjects := collectClusterOperatorRelatedObjects(
				tt.args.ctx,
				tt.args.dynamicClient,
				tt.args.co,
				tt.args.resVer)
			assert.Len(t, actualClusterOperatorRelObjects, 1)
			assert.Equal(t, tt.want, actualClusterOperatorRelObjects)
		})
	}
}

func Test_Operators_GetOperatorResourcesVersions(t *testing.T) {
	type args struct {
		discoveryClient discovery.DiscoveryInterface
	}
	tests := []struct {
		name    string
		args    args
		want    map[schema.GroupResource]string
		wantErr bool
	}{
		{
			name:    "empty operator resources versions",
			args:    args{discoveryClient: kubefake.NewSimpleClientset().Discovery()},
			want:    map[schema.GroupResource]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := getOperatorResourcesVersions(tt.args.discoveryClient)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOperatorResourcesVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getOperatorResourcesVersions() = %v, want %v", got, tt.want)
			}
		})
	}
}
