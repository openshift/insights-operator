package clusterconfig

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	openshiftscheme "github.com/openshift/client-go/config/clientset/versioned/scheme"
	"github.com/openshift/insights-operator/pkg/record"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/utils/pointer"
)

func newClusterOperator() configv1.ClusterOperator {
	return configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusteroperator",
		},
	}
}

func newConfigClientset(t *testing.T) *configfake.Clientset {
	testOperator := newClusterOperator()
	configCS := configfake.NewSimpleClientset()
	_, err := configCS.ConfigV1().ClusterOperators().Create(context.Background(), &testOperator, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake clusteroperator", err)
	}
	return configCS
}

func Test_Operators_GatherClusterOperators(t *testing.T) {
	cfg := newConfigClientset(t)

	records, err := gatherClusterOperators(context.Background(), cfg.ConfigV1(), kubefake.NewSimpleClientset().CoreV1(), cfg.Discovery(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()))
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

func Test_Operators_GatherPodContainersLogs(t *testing.T) {
	type args struct {
		ctx        context.Context
		client     corev1client.CoreV1Interface
		pods       []*corev1.Pod
		bufferSize int64
	}
	tests := []struct {
		name    string
		args    args
		want    []record.Record
		wantErr bool
	}{
		{
			name: "total container is zero and the podlist is empty",
			args: args{
				ctx:        context.TODO(),
				client:     kubefake.NewSimpleClientset().CoreV1(),
				pods:       []*v1.Pod{},
				bufferSize: 0,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "total container is two and the podlist is empty",
			args: args{
				ctx:        context.TODO(),
				client:     kubefake.NewSimpleClientset().CoreV1(),
				pods:       []*v1.Pod{},
				bufferSize: int64(10 * 10 / 2 / 2),
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gatherPodContainersLogs(tt.args.ctx, tt.args.client, tt.args.pods, tt.args.bufferSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("gatherNamespaceEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherPodContainersLogs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_Operators_GetContainerLogs(t *testing.T) {
	bufferSize := int64(8 * 1024 * 1024 * logCompressionRatio / 10 / 2)

	type args struct {
		ctx        context.Context
		client     corev1client.CoreV1Interface
		pod        *corev1.Pod
		isPrevious bool
		buf        *bytes.Buffer
		bufferSize int64
	}
	tests := []struct {
		name string
		args args
		want []record.Record
	}{
		{
			name: "empty pod containers log",
			args: args{
				ctx:        context.TODO(),
				client:     kubefake.NewSimpleClientset().CoreV1(),
				pod:        &v1.Pod{},
				isPrevious: false,
				buf:        bytes.NewBuffer(make([]byte, 0, bufferSize)),
				bufferSize: bufferSize,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getContainerLogs(tt.args.ctx, tt.args.client, tt.args.pod, tt.args.isPrevious, tt.args.buf, tt.args.bufferSize); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getContainerLogs() = %v, want %v", got, tt.want)
			}
		})
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
			if got := clusterOperatorsRecords(tt.args.ctx, tt.args.items, tt.args.dynamicClient, tt.args.discoveryClient); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clusterOperatorsRecords() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_Operators_GatherUnhealthyClusterOperator(t *testing.T) {
	type args struct {
		ctx        context.Context
		items      []configv1.ClusterOperator
		coreClient corev1client.CoreV1Interface
	}
	tests := []struct {
		name  string
		args  args
		want  []*corev1.Pod
		want1 []record.Record
		want2 int
	}{
		{
			name: "test empty list",
			args: args{
				ctx:        context.TODO(),
				items:      []configv1.ClusterOperator{},
				coreClient: kubefake.NewSimpleClientset().CoreV1(),
			},
			want:  []*v1.Pod{},
			want1: nil,
			want2: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2 := gatherUnhealthyClusterOperator(tt.args.ctx, tt.args.items, tt.args.coreClient)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherUnhealthyClusterOperator() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("gatherUnhealthyClusterOperator() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("gatherUnhealthyClusterOperator() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func Test_Operators_GatherUnhealthyPods(t *testing.T) {
	type args struct {
		pods []corev1.Pod
	}
	tests := []struct {
		name  string
		args  args
		want  []*corev1.Pod
		want1 []record.Record
		want2 int
	}{
		{
			name:  "empty pod list",
			args:  args{pods: []v1.Pod{}},
			want:  nil,
			want1: nil,
			want2: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2 := gatherUnhealthyPods(tt.args.pods)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherUnhealthyPods() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("gatherUnhealthyPods() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("gatherUnhealthyPods() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func Test_Operators_GatherNamespaceEvents(t *testing.T) {
	type args struct {
		ctx        context.Context
		coreClient corev1client.CoreV1Interface
		namespace  string
	}
	tests := []struct {
		name    string
		args    args
		want    []record.Record
		wantErr bool
	}{
		{
			name: "empty namespace events",
			args: args{
				ctx:        context.TODO(),
				coreClient: kubefake.NewSimpleClientset().CoreV1(),
				namespace:  "insights-operator",
			},
			want:    []record.Record{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := gatherNamespaceEvents(tt.args.ctx, tt.args.coreClient, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("gatherNamespaceEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("gatherNamespaceEvents() = %v, want %v", got, tt.want)
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
			if got := collectClusterOperatorResources(tt.args.ctx, tt.args.dynamicClient, tt.args.co, tt.args.resVer); !reflect.DeepEqual(got, tt.want) {
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

func Test_Operators_FetchPodContainerLog(t *testing.T) {
	bufferSize := int64(8 * 1024 * 1024 * logCompressionRatio / 10 / 2)

	type args struct {
		ctx           context.Context
		coreClient    corev1client.CoreV1Interface
		pod           *corev1.Pod
		buf           *bytes.Buffer
		containerName string
		isPrevious    bool
		maxBytes      *int64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "container without previous log",
			args: args{
				ctx:           context.TODO(),
				coreClient:    kubefake.NewSimpleClientset().CoreV1(),
				pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "testPod"}},
				buf:           bytes.NewBuffer(make([]byte, 0, bufferSize)),
				containerName: "testContainer",
				isPrevious:    false,
				maxBytes:      pointer.Int64Ptr(bufferSize),
			},
			wantErr: false,
		},
		{
			name: "container with previous log",
			args: args{
				ctx:           context.TODO(),
				coreClient:    kubefake.NewSimpleClientset().CoreV1(),
				pod:           &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "testPod"}},
				buf:           bytes.NewBuffer(make([]byte, 0, bufferSize)),
				containerName: "testContainer",
				isPrevious:    true,
				maxBytes:      pointer.Int64Ptr(bufferSize),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := fetchPodContainerLog(tt.args.ctx, tt.args.coreClient, tt.args.pod, tt.args.buf, tt.args.containerName, tt.args.isPrevious, tt.args.maxBytes); (err != nil) != tt.wantErr {
				t.Errorf("fetchPodContainerLog() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_Operators_IsHealthyOperator(t *testing.T) {
	type args struct {
		operator *configv1.ClusterOperator
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "cluster operator isn't degraded",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
						{Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse},
					}},
				},
			},
			want: true,
		},
		{
			name: "cluster operator is available",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
						{Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue},
					}},
				},
			},
			want: true,
		},
		{
			name: "cluster operator is degraded",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
						{Type: configv1.OperatorDegraded, Status: configv1.ConditionTrue},
					}},
				},
			},
			want: false,
		},
		{
			name: "cluster operator isn't available",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{Conditions: []configv1.ClusterOperatorStatusCondition{
						{Type: configv1.OperatorAvailable, Status: configv1.ConditionFalse},
					}},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isHealthyOperator(tt.args.operator); got != tt.want {
				t.Errorf("isHealthyOperator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_Operators_IsPodRestarted(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "pod isn't restarted with InitStatuses",
			args: args{
				pod: &v1.Pod{
					Status: v1.PodStatus{
						InitContainerStatuses: []v1.ContainerStatus{
							v1.ContainerStatus{RestartCount: 0},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod was restarted with InitStatuses",
			args: args{
				pod: &v1.Pod{
					Status: v1.PodStatus{
						InitContainerStatuses: []v1.ContainerStatus{
							v1.ContainerStatus{RestartCount: 2},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod isn't restarted",
			args: args{
				pod: &v1.Pod{
					Status: v1.PodStatus{
						ContainerStatuses: []v1.ContainerStatus{
							v1.ContainerStatus{RestartCount: 0},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod was restarted",
			args: args{
				pod: &v1.Pod{
					Status: v1.PodStatus{
						ContainerStatuses: []v1.ContainerStatus{
							v1.ContainerStatus{RestartCount: 2},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPodRestarted(tt.args.pod); got != tt.want {
				t.Errorf("isPodRestarted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_Operators_NamespacesForOperator(t *testing.T) {
	type args struct {
		operator *configv1.ClusterOperator
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "Cluster operator with one namespace",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{
						RelatedObjects: []configv1.ObjectReference{
							{Group: "", Resource: "namespaces", Name: "namespace1"},
						},
					},
				},
			},
			want: []string{"namespace1"},
		},
		{
			name: "Cluster operator with more than one namespace",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{
						RelatedObjects: []configv1.ObjectReference{
							{Group: "", Resource: "namespaces", Name: "namespace1"},
							{Group: "", Resource: "namespaces", Name: "namespace2"},
							{Group: "", Resource: "not-namespaces", Name: "not-namespace"},
						},
					},
				},
			},
			want: []string{"namespace1", "namespace2"},
		},
		{
			name: "Cluster operator without namespace",
			args: args{
				operator: &configv1.ClusterOperator{
					ObjectMeta: metav1.ObjectMeta{
						Name: "insights",
					},
					Status: configv1.ClusterOperatorStatus{
						RelatedObjects: []configv1.ObjectReference{},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := namespacesForOperator(tt.args.operator); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("namespacesForOperator() = %v, want %v", got, tt.want)
			}
		})
	}
}
