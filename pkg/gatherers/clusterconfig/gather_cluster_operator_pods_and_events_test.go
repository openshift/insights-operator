package clusterconfig

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/utils/pointer"

	"github.com/openshift/insights-operator/pkg/record"
)

func Test_UnhealtyOperators_GatherClusterOperatorPodsAndEvents(t *testing.T) {
	testOperator := configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusteroperator",
		},
	}
	cfg := configfake.NewSimpleClientset()
	_, err := cfg.ConfigV1().ClusterOperators().Create(context.Background(), &testOperator, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake clusteroperator", err)
	}

	_, err = gatherClusterOperatorPodsAndEvents(context.Background(), cfg.ConfigV1(), kubefake.NewSimpleClientset().CoreV1(), 1*time.Minute)
	if err != nil {
		t.Errorf("unexpected errors: %#v", err)
		return
	}
}

func Test_UnhealtyOperators_GatherPodContainersLogs(t *testing.T) {
	type args struct {
		ctx        context.Context
		client     corev1client.CoreV1Interface
		pods       []*v1.Pod
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
			got, err := gatherPodsAndTheirContainersLogs(tt.args.ctx, tt.args.client, tt.args.pods, tt.args.bufferSize)
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

func Test_UnhealtyOperators_GetContainerLogs(t *testing.T) {
	bufferSize := int64(8 * 1024 * 1024 * logCompressionRatio / 10 / 2)

	type args struct {
		ctx        context.Context
		client     corev1client.CoreV1Interface
		pod        *v1.Pod
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
			if got := getContainerLogs(
				tt.args.ctx,
				tt.args.client,
				tt.args.pod,
				tt.args.isPrevious,
				tt.args.buf); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getContainerLogs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_UnhealtyOperators_UnhealthyClusterOperator(t *testing.T) {
	type args struct {
		ctx        context.Context
		items      []configv1.ClusterOperator
		coreClient corev1client.CoreV1Interface
	}
	tests := []struct {
		name  string
		args  args
		want  []*v1.Pod
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
			got, got1, got2 := unhealthyClusterOperator(tt.args.ctx, tt.args.items, tt.args.coreClient, 1*time.Minute)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unhealthyClusterOperator() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("unhealthyClusterOperator() got1 = %v, want1 %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("unhealthyClusterOperator() got2 = %v, want2 %v", got2, tt.want2)
			}
		})
	}
}

func Test_UnhealtyOperators_GatherUnhealthyPods(t *testing.T) {
	type args struct {
		pods []v1.Pod
	}
	tests := []struct {
		name  string
		args  args
		want  []*v1.Pod
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
			got, got2 := getAllRelatedPods(tt.args.pods)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAllRelatedPods() got = %v, want %v", got, tt.want)
			}
			if got2 != tt.want2 {
				t.Errorf("getAllRelatedPods() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func Test_UnhealtyOperators_GatherNamespaceEvents(t *testing.T) {
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
			got, err := gatherNamespaceEvents(tt.args.ctx, tt.args.coreClient, tt.args.namespace, 1*time.Minute)
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

func Test_UnhealtyOperators_FetchPodContainerLog(t *testing.T) {
	bufferSize := int64(8 * 1024 * 1024 * logCompressionRatio / 10 / 2)

	type args struct {
		ctx           context.Context
		coreClient    corev1client.CoreV1Interface
		pod           *v1.Pod
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
				maxBytes:      pointer.Int64(bufferSize),
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
				maxBytes:      pointer.Int64(bufferSize),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := fetchPodContainerLog(
				tt.args.ctx,
				tt.args.coreClient,
				tt.args.pod,
				tt.args.buf,
				tt.args.containerName,
				tt.args.isPrevious,
				tt.args.maxBytes); (err != nil) != tt.wantErr {
				t.Errorf("fetchPodContainerLog() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_UnhealtyOperators_IsHealthyOperator(t *testing.T) {
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

func Test_UnhealtyOperators_IsPodRestarted(t *testing.T) {
	type args struct {
		pod *v1.Pod
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
							{RestartCount: 0},
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
							{RestartCount: 2},
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
							{RestartCount: 0},
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
							{RestartCount: 2},
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

func Test_UnhealtyOperators_NamespacesForOperator(t *testing.T) {
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

func Test_getLogWithStacktracing(t *testing.T) {
	shortenedLogs := getLogWithStacktracing(strings.Split(logWithShortStacktrace, "\n"))
	assert.Equal(t, shortenedLogs, logWithShortStacktrace)

	shortenedLogs = getLogWithStacktracing(strings.Split(logWithLongStacktrace, "\n"))
	assert.Equal(t, shortenedLogs, shortenedLogWithLongStacktrace)
}

//nolint:lll
const logWithShortStacktrace = `2021-07-19T13:55:45.602876459Z I0719 13:55:45.602129       1 observer_polling.go:111] Observed file "/var/run/secrets/serving-cert/tls.crt" has been created (hash="00c073d4ec979fbfa3f19d54147bb181b91ae3ef50f5e1708b3adc85118fc52a")
2021-07-19T13:55:45.602876459Z W0719 13:55:45.602241       1 builder.go:120] Restart triggered because of file /var/run/secrets/serving-cert/tls.crt was created
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602424       1 configmap_cafile_content.go:223] Shutting down client-ca::kube-system::extension-apiserver-authentication::client-ca-file
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602444       1 configmap_cafile_content.go:223] Shutting down client-ca::kube-system::extension-apiserver-authentication::requestheader-client-ca-file
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602451       1 requestheader_controller.go:183] Shutting down RequestHeaderAuthRequestController
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602524       1 tlsconfig.go:255] Shutting down DynamicServingCertificateController
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602533       1 dynamic_serving_content.go:145] Shutting down serving-cert::/tmp/serving-cert-115806414/tls.crt::/tmp/serving-cert-115806414/tls.key
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602628       1 secure_serving.go:241] Stopped listening on [::]:8443
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602646       1 observer_polling.go:111] Observed file "/var/run/secrets/serving-cert/tls.key" has been created (hash="e445aa35113893d50bcb95a60123911d6c8f10f2e20d14caa5622ecdbdb6beb1")
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602819       1 observer_polling.go:111] Observed file "/var/run/configmaps/service-ca-bundle/service-ca.crt" has been created (hash="d0e5dd708098dec731ccc2ed019572bb64b08ce94d8d6ffaf8e43562131f2cd7")
2021-07-19T13:55:46.102917367Z I0719 13:55:46.102726       1 builder.go:263] server exited
2021-07-19T13:55:50.601611099Z I0719 13:55:50.601077       1 observer_polling.go:162] Shutting down file observer
2021-07-19T13:55:54.463324877Z I0719 13:55:54.463217       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/server/dynamiccertificates/configmap_cafile_content.go:206
2021-07-19T13:55:54.513213337Z I0719 13:55:54.513128       1 reflector.go:530] k8s.io/apiserver/pkg/authentication/request/headerrequest/requestheader_controller.go:172: Watch close - *v1.ConfigMap total 0 items received
2021-07-19T13:55:54.513213337Z I0719 13:55:54.513196       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/authentication/request/headerrequest/requestheader_controller.go:172
2021-07-19T13:55:54.513298910Z I0719 13:55:54.513214       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/server/dynamiccertificates/configmap_cafile_content.go:206
2021-07-19T13:56:01.508364946Z I0719 13:56:01.505044       1 operator.go:135] Unable to check insights-operator pod status. Setting initial delay to 14m10.324303634s
2021-07-19T13:56:01.508364946Z F0719 13:56:01.505679       1 start.go:86] unable to set initial cluster status: context canceled
2021-07-19T13:56:01.508364946Z goroutine 1 [running]:
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.stacks(0xc000012001, 0xc0009b0240, 0x62, 0x10d)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1026 +0xb9
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).output(0x2fd7ea0, 0xc000000003, 0x0, 0x0, 0xc0009a5ea0, 0x2ef769e, 0x8, 0x56, 0x414300)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:975 +0x19b
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).printDepth(0x2fd7ea0, 0xc000000003, 0x0, 0x0, 0x0, 0x0, 0x1, 0xc000aac4b0, 0x1, 0x1)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:732 +0x16f
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).print(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:714
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.Fatal(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1482
2021-07-19T13:56:01.508364946Z github.com/openshift/insights-operator/pkg/cmd/start.NewOperator.func1(0xc0009a9080, 0xc0005253a0, 0x0, 0x2)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/pkg/cmd/start/start.go:86 +0xac5
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).execute(0xc0009a9080, 0xc000525380, 0x2, 0x2, 0xc0009a9080, 0xc000525380)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:854 +0x2c2
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).ExecuteC(0xc0009a8dc0, 0x1f5ab86, 0x4, 0x0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:958 +0x375
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).Execute(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:895
2021-07-19T13:56:01.508364946Z main.main()
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/cmd/insights-operator/main.go:25 +0xf9
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 6 [chan receive]:
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).flushDaemon(0x2fd7ea0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1169 +0x8b
2021-07-19T13:56:01.508364946Z created by k8s.io/klog/v2.init.0
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:417 +0xdf
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 359 [runnable]:
2021-07-19T13:56:01.508364946Z github.com/openshift/insights-operator/pkg/controller/periodic.(*Controller).Run(0xc0007329c0, 0xc0000aac60, 0xc5fb472f12)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/pkg/controller/periodic/periodic.go:60
2021-07-19T13:56:01.508364946Z created by github.com/openshift/insights-operator/pkg/controller.(*Support).Run
`

//nolint:lll
const logWithLongStacktrace = `2021-07-19T13:55:45.602876459Z I0719 13:55:45.602129       1 observer_polling.go:111] Observed file "/var/run/secrets/serving-cert/tls.crt" has been created (hash="00c073d4ec979fbfa3f19d54147bb181b91ae3ef50f5e1708b3adc85118fc52a")
2021-07-19T13:55:45.602876459Z W0719 13:55:45.602241       1 builder.go:120] Restart triggered because of file /var/run/secrets/serving-cert/tls.crt was created
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602424       1 configmap_cafile_content.go:223] Shutting down client-ca::kube-system::extension-apiserver-authentication::client-ca-file
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602444       1 configmap_cafile_content.go:223] Shutting down client-ca::kube-system::extension-apiserver-authentication::requestheader-client-ca-file
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602451       1 requestheader_controller.go:183] Shutting down RequestHeaderAuthRequestController
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602524       1 tlsconfig.go:255] Shutting down DynamicServingCertificateController
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602533       1 dynamic_serving_content.go:145] Shutting down serving-cert::/tmp/serving-cert-115806414/tls.crt::/tmp/serving-cert-115806414/tls.key
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602628       1 secure_serving.go:241] Stopped listening on [::]:8443
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602646       1 observer_polling.go:111] Observed file "/var/run/secrets/serving-cert/tls.key" has been created (hash="e445aa35113893d50bcb95a60123911d6c8f10f2e20d14caa5622ecdbdb6beb1")
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602819       1 observer_polling.go:111] Observed file "/var/run/configmaps/service-ca-bundle/service-ca.crt" has been created (hash="d0e5dd708098dec731ccc2ed019572bb64b08ce94d8d6ffaf8e43562131f2cd7")
2021-07-19T13:55:46.102917367Z I0719 13:55:46.102726       1 builder.go:263] server exited
2021-07-19T13:55:50.601611099Z I0719 13:55:50.601077       1 observer_polling.go:162] Shutting down file observer
2021-07-19T13:55:54.463324877Z I0719 13:55:54.463217       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/server/dynamiccertificates/configmap_cafile_content.go:206
2021-07-19T13:55:54.513213337Z I0719 13:55:54.513128       1 reflector.go:530] k8s.io/apiserver/pkg/authentication/request/headerrequest/requestheader_controller.go:172: Watch close - *v1.ConfigMap total 0 items received
2021-07-19T13:55:54.513213337Z I0719 13:55:54.513196       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/authentication/request/headerrequest/requestheader_controller.go:172
2021-07-19T13:55:54.513298910Z I0719 13:55:54.513214       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/server/dynamiccertificates/configmap_cafile_content.go:206
2021-07-19T13:56:01.508364946Z I0719 13:56:01.505044       1 operator.go:135] Unable to check insights-operator pod status. Setting initial delay to 14m10.324303634s
2021-07-19T13:56:01.508364946Z F0719 13:56:01.505679       1 start.go:86] unable to set initial cluster status: context canceled
2021-07-19T13:56:01.508364946Z goroutine 1 [running]:
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.stacks(0xc000012001, 0xc0009b0240, 0x62, 0x10d)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1026 +0xb9
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).output(0x2fd7ea0, 0xc000000003, 0x0, 0x0, 0xc0009a5ea0, 0x2ef769e, 0x8, 0x56, 0x414300)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:975 +0x19b
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).printDepth(0x2fd7ea0, 0xc000000003, 0x0, 0x0, 0x0, 0x0, 0x1, 0xc000aac4b0, 0x1, 0x1)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:732 +0x16f
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).print(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:714
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.Fatal(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1482
2021-07-19T13:56:01.508364946Z github.com/openshift/insights-operator/pkg/cmd/start.NewOperator.func1(0xc0009a9080, 0xc0005253a0, 0x0, 0x2)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/pkg/cmd/start/start.go:86 +0xac5
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).execute(0xc0009a9080, 0xc000525380, 0x2, 0x2, 0xc0009a9080, 0xc000525380)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:854 +0x2c2
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).ExecuteC(0xc0009a8dc0, 0x1f5ab86, 0x4, 0x0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:958 +0x375
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).Execute(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:895
2021-07-19T13:56:01.508364946Z main.main()
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/cmd/insights-operator/main.go:25 +0xf9
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 6 [chan receive]:
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).flushDaemon(0x2fd7ea0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1169 +0x8b
2021-07-19T13:56:01.508364946Z created by k8s.io/klog/v2.init.0
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:417 +0xdf
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 359 [runnable]:
2021-07-19T13:56:01.508364946Z github.com/openshift/insights-operator/pkg/controller/periodic.(*Controller).Run(0xc0007329c0, 0xc0000aac60, 0xc5fb472f12)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/pkg/controller/periodic/periodic.go:60
2021-07-19T13:56:01.508364946Z created by github.com/openshift/insights-operator/pkg/controller.(*Support).Run
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/pkg/controller/operator.go:137 +0x99f
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 102 [select]:
2021-07-19T13:56:01.508364946Z k8s.io/apimachinery/pkg/util/wait.BackoffUntil(0x20aabc0, 0x22160e0, 0xc0007c8000, 0x1, 0xc0000aa0c0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/apimachinery/pkg/util/wait/wait.go:167 +0x149
2021-07-19T13:56:01.508364946Z k8s.io/apimachinery/pkg/util/wait.JitterUntil(0x20aabc0, 0x12a05f200, 0x0, 0xc000997b01, 0xc0000aa0c0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/apimachinery/pkg/util/wait/wait.go:133 +0x98
2021-07-19T13:56:01.508364946Z k8s.io/apimachinery/pkg/util/wait.Until(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/apimachinery/pkg/util/wait/wait.go:90
2021-07-19T13:56:01.508364946Z k8s.io/apimachinery/pkg/util/wait.Forever(0x20aabc0, 0x12a05f200)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/apimachinery/pkg/util/wait/wait.go:81 +0x4f
2021-07-19T13:56:01.508364946Z created by k8s.io/component-base/logs.InitLogs
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/component-base/logs/logs.go:58 +0x8a
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 108 [IO wait]:
2021-07-19T13:56:01.508364946Z internal/poll.runtime_pollWait(0x7f3c9d62eb70, 0x72, 0x2218b20)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/runtime/netpoll.go:222 +0x55
2021-07-19T13:56:01.508364946Z internal/poll.(*pollDesc).wait(0xc000bb4298, 0x72, 0x2218b00, 0x2f2c9c0, 0x0)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/internal/poll/fd_poll_runtime.go:87 +0x45
2021-07-19T13:56:01.508364946Z internal/poll.(*pollDesc).waitRead(...)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/internal/poll/fd_poll_runtime.go:92
2021-07-19T13:56:01.508364946Z internal/poll.(*FD).Read(0xc000bb4280, 0xc0006ab000, 0x6b31, 0x6b31, 0x0, 0x0, 0x0)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/internal/poll/fd_unix.go:159 +0x1a5
2021-07-19T13:56:01.508364946Z net.(*netFD).Read(0xc000bb4280, 0xc0006ab000, 0x6b31, 0x6b31, 0x203000, 0x6e349b, 0xc0000babe0)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/net/fd_posix.go:55 +0x4f
2021-07-19T13:56:01.508364946Z net.(*conn).Read(0xc0000122b8, 0xc0006ab000, 0x6b31, 0x6b31, 0x0, 0x0, 0x0)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/net/net.go:182 +0x8e
2021-07-19T13:56:01.508364946Z crypto/tls.(*atLeastReader).Read(0xc0007d0360, 0xc0006ab000, 0x6b31, 0x6b31, 0x1a, 0x6b0d, 0xc0009ef710)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/crypto/tls/conn.go:779 +0x62
2021-07-19T13:56:01.508364946Z bytes.(*Buffer).ReadFrom(0xc0000bad00, 0x22144a0, 0xc0007d0360, 0x411785, 0x1ce2c20, 0x1eccc00)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/bytes/buffer.go:204 +0xb1
2021-07-19T13:56:01.508364946Z crypto/tls.(*Conn).readFromUntil(0xc0000baa80, 0x2216a20, 0xc0000122b8, 0x5, 0xc0000122b8, 0x9)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/crypto/tls/conn.go:801 +0xf3
2021-07-19T13:56:01.508364946Z crypto/tls.(*Conn).readRecordOrCCS(0xc0000baa80, 0x0, 0x0, 0xc0009efd18)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/crypto/tls/conn.go:608 +0x115
2021-07-19T13:56:01.508364946Z crypto/tls.(*Conn).readRecord(...)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/crypto/tls/conn.go:576
2021-07-19T13:56:01.508364946Z crypto/tls.(*Conn).Read(0xc0000baa80, 0xc000a16000, 0x1000, 0x1000, 0x0, 0x0, 0x0)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/crypto/tls/conn.go:1252 +0x15f
2021-07-19T13:56:01.508364946Z bufio.(*Reader).Read(0xc0002775c0, 0xc000855378, 0x9, 0x9, 0xc0009efd18, 0x20ab700, 0xa212cb)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/bufio/bufio.go:227 +0x222
2021-07-19T13:56:01.508364946Z io.ReadAtLeast(0x22142e0, 0xc0002775c0, 0xc000855378, 0x9, 0x9, 0x9, 0xc00007a060, 0x0, 0x22146c0)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/io/io.go:314 +0x87
2021-07-19T13:56:01.508364946Z io.ReadFull(...)
2021-07-19T13:56:01.508364946Z 	/usr/lib/golang/src/io/io.go:333
2021-07-19T13:56:01.508364946Z golang.org/x/net/http2.readFrameHeader(0xc000855378, 0x9, 0x9, 0x22142e0, 0xc0002775c0, 0x0, 0x0, 0xc0009efdd0, 0x4736e5)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/golang.org/x/net/http2/frame.go:237 +0x89
2021-07-19T13:56:01.508364946Z golang.org/x/net/http2.(*Framer).ReadFrame(0xc000855340, 0xc000b021b0, 0x0, 0x0, 0x0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/golang.org/x/net/http2/frame.go:492 +0xa5
2021-07-19T13:56:01.508364946Z golang.org/x/net/http2.(*clientConnReadLoop).run(0xc0009effa8, 0x0, 0x0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/golang.org/x/net/http2/transport.go:1819 +0xd8
2021-07-19T13:56:01.508364946Z golang.org/x/net/http2.(*ClientConn).readLoop(0xc000102a80)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/golang.org/x/net/http2/transport.go:1741 +0x6f
2021-07-19T13:56:01.508364946Z created by golang.org/x/net/http2.(*Transport).newClientConn
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/golang.org/x/net/http2/transport.go:705 +0x6c5
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 131 [chan receive, 1 minutes]:
2021-07-19T13:56:01.508364946Z k8s.io/apimachinery/pkg/watch.(*Broadcaster).loop(0xc00061cbc0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/apimachinery/pkg/watch/mux.go:219 +0x66
2021-07-19T13:56:01.508364946Z created by k8s.io/apimachinery/pkg/watch.NewBroadcaster
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/apimachinery/pkg/watch/mux.go:73 +0xf7
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 132 [chan receive, 1 minutes]:
2021-07-19T13:56:01.508364946Z k8s.io/client-go/tools/record.(*eventBroadcasterImpl).StartEventWatcher.func1(0x222a600, 0xc000b03770, 0xc000834800)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/client-go/tools/record/event.go:301 +0xaa
2021-07-19T13:56:01.508364946Z created by k8s.io/client-go/tools/record.(*eventBroadcasterImpl).StartEventWatcher
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/client-go/tools/record/event.go:299 +0x6e
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 133 [chan receive, 1 minutes]:
2021-07-19T13:56:01.508364946Z k8s.io/client-go/tools/record.(*eventBroadcasterImpl).StartEventWatcher.func1(0x222a600, 0xc000b03920, 0xc000b038f0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/client-go/tools/record/event.go:301 +0xaa
2021-07-19T13:56:01.508364946Z created by k8s.io/client-go/tools/record.(*eventBroadcasterImpl).StartEventWatcher
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/client-go/tools/record/event.go:299 +0x6e
`

//nolint:lll
const shortenedLogWithLongStacktrace = `2021-07-19T13:55:45.602876459Z I0719 13:55:45.602129       1 observer_polling.go:111] Observed file "/var/run/secrets/serving-cert/tls.crt" has been created (hash="00c073d4ec979fbfa3f19d54147bb181b91ae3ef50f5e1708b3adc85118fc52a")
2021-07-19T13:55:45.602876459Z W0719 13:55:45.602241       1 builder.go:120] Restart triggered because of file /var/run/secrets/serving-cert/tls.crt was created
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602424       1 configmap_cafile_content.go:223] Shutting down client-ca::kube-system::extension-apiserver-authentication::client-ca-file
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602444       1 configmap_cafile_content.go:223] Shutting down client-ca::kube-system::extension-apiserver-authentication::requestheader-client-ca-file
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602451       1 requestheader_controller.go:183] Shutting down RequestHeaderAuthRequestController
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602524       1 tlsconfig.go:255] Shutting down DynamicServingCertificateController
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602533       1 dynamic_serving_content.go:145] Shutting down serving-cert::/tmp/serving-cert-115806414/tls.crt::/tmp/serving-cert-115806414/tls.key
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602628       1 secure_serving.go:241] Stopped listening on [::]:8443
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602646       1 observer_polling.go:111] Observed file "/var/run/secrets/serving-cert/tls.key" has been created (hash="e445aa35113893d50bcb95a60123911d6c8f10f2e20d14caa5622ecdbdb6beb1")
2021-07-19T13:55:45.602876459Z I0719 13:55:45.602819       1 observer_polling.go:111] Observed file "/var/run/configmaps/service-ca-bundle/service-ca.crt" has been created (hash="d0e5dd708098dec731ccc2ed019572bb64b08ce94d8d6ffaf8e43562131f2cd7")
2021-07-19T13:55:46.102917367Z I0719 13:55:46.102726       1 builder.go:263] server exited
2021-07-19T13:55:50.601611099Z I0719 13:55:50.601077       1 observer_polling.go:162] Shutting down file observer
2021-07-19T13:55:54.463324877Z I0719 13:55:54.463217       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/server/dynamiccertificates/configmap_cafile_content.go:206
2021-07-19T13:55:54.513213337Z I0719 13:55:54.513128       1 reflector.go:530] k8s.io/apiserver/pkg/authentication/request/headerrequest/requestheader_controller.go:172: Watch close - *v1.ConfigMap total 0 items received
2021-07-19T13:55:54.513213337Z I0719 13:55:54.513196       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/authentication/request/headerrequest/requestheader_controller.go:172
2021-07-19T13:55:54.513298910Z I0719 13:55:54.513214       1 reflector.go:225] Stopping reflector *v1.ConfigMap (12h0m0s) from k8s.io/apiserver/pkg/server/dynamiccertificates/configmap_cafile_content.go:206
2021-07-19T13:56:01.508364946Z I0719 13:56:01.505044       1 operator.go:135] Unable to check insights-operator pod status. Setting initial delay to 14m10.324303634s
2021-07-19T13:56:01.508364946Z F0719 13:56:01.505679       1 start.go:86] unable to set initial cluster status: context canceled
2021-07-19T13:56:01.508364946Z goroutine 1 [running]:
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.stacks(0xc000012001, 0xc0009b0240, 0x62, 0x10d)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1026 +0xb9
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).output(0x2fd7ea0, 0xc000000003, 0x0, 0x0, 0xc0009a5ea0, 0x2ef769e, 0x8, 0x56, 0x414300)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:975 +0x19b
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).printDepth(0x2fd7ea0, 0xc000000003, 0x0, 0x0, 0x0, 0x0, 0x1, 0xc000aac4b0, 0x1, 0x1)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:732 +0x16f
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.(*loggingT).print(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:714
2021-07-19T13:56:01.508364946Z k8s.io/klog/v2.Fatal(...)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/klog/v2/klog.go:1482
2021-07-19T13:56:01.508364946Z github.com/openshift/insights-operator/pkg/cmd/start.NewOperator.func1(0xc0009a9080, 0xc0005253a0, 0x0, 0x2)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/pkg/cmd/start/start.go:86 +0xac5
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).execute(0xc0009a9080, 0xc000525380, 0x2, 0x2, 0xc0009a9080, 0xc000525380)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:854 +0x2c2
2021-07-19T13:56:01.508364946Z github.com/spf13/cobra.(*Command).ExecuteC(0xc0009a8dc0, 0x1f5ab86, 0x4, 0x0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/github.com/spf13/cobra/command.go:958 +0x375
... (62 stacktrace lines suppressed) ...
2021-07-19T13:56:01.508364946Z
2021-07-19T13:56:01.508364946Z goroutine 133 [chan receive, 1 minutes]:
2021-07-19T13:56:01.508364946Z k8s.io/client-go/tools/record.(*eventBroadcasterImpl).StartEventWatcher.func1(0x222a600, 0xc000b03920, 0xc000b038f0)
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/client-go/tools/record/event.go:301 +0xaa
2021-07-19T13:56:01.508364946Z created by k8s.io/client-go/tools/record.(*eventBroadcasterImpl).StartEventWatcher
2021-07-19T13:56:01.508364946Z 	/go/src/github.com/openshift/insights-operator/vendor/k8s.io/client-go/tools/record/event.go:299 +0x6e
`
