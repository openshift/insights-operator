package clusterconfig

import (
	"context"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	openshiftscheme "github.com/openshift/client-go/config/clientset/versioned/scheme"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func newClusterOperator() configv1.ClusterOperator {
	return configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusteroperator",
		},
	}
}

func Test_Operators_GatherClusterOperators(t *testing.T) {
	testOperator := newClusterOperator()
	cfg := configfake.NewSimpleClientset()
	_, err := cfg.ConfigV1().ClusterOperators().Create(context.Background(), &testOperator, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake clusteroperator", err)
	}

	records, err := gatherClusterOperators(
		context.Background(),
		cfg.ConfigV1(),
		cfg.Discovery(),
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
	)
	if err != nil {
		t.Errorf("unexpected errors: %#v", err)
		return
	}

	item, _ := records[0].Item.Marshal(context.TODO())
	var gatheredCO configv1.ClusterOperator
	openshiftCodec := openshiftscheme.Codecs.LegacyCodec(configv1.SchemeGroupVersion)
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

func Test_Operators_CollectClusterOperatorResources(t *testing.T) {
	type args struct {
		ctx           context.Context
		dynamicClient dynamic.Interface
		co            configv1.ClusterOperator
		resVer        map[string][]string
	}
	tests := []struct {
		name string
		args args
		want []clusterOperatorResource
	}{
		{
			name: "empty cluster operator resources",
			args: args{
				ctx:           context.TODO(),
				dynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
				co:            newClusterOperator(),
				resVer:        map[string][]string{},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectClusterOperatorResources(
				tt.args.ctx,
				tt.args.dynamicClient,
				tt.args.co,
				tt.args.resVer); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectClusterOperatorResources() = %v, want %v", got, tt.want)
			}
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
		want    map[string][]string
		wantErr bool
	}{
		{
			name:    "empty operator resources versions",
			args:    args{discoveryClient: kubefake.NewSimpleClientset().Discovery()},
			want:    map[string][]string{},
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
