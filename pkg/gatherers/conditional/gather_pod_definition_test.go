package conditional

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func TestGatherer_gatherPodDefinition(t *testing.T) {
	type fields struct {
		firingAlerts map[string][]AlertLabels
	}
	type args struct {
		ctx        context.Context
		params     GatherPodDefinitionParams
		coreClient func(context.Context) v1.CoreV1Interface
	}
	tests := []struct {
		name    string
		args    args
		fields  fields
		wantLen int
		wantErr bool
	}{
		{
			name: "get pod definition",
			args: args{
				ctx: context.TODO(),
				params: GatherPodDefinitionParams{
					AlertName: "KubePodNotReady",
				},
				coreClient: func(ctx context.Context) v1.CoreV1Interface {
					coreClient := kubefake.NewClientset().CoreV1()
					_, err := coreClient.Pods("ns").Create(ctx, &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "ns",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodPending,
						},
						Spec: corev1.PodSpec{},
					}, metav1.CreateOptions{})
					if err != nil {
						t.Fatalf("unable to create fake pod: %v", err)
					}
					return coreClient
				},
			},
			fields: fields{
				firingAlerts: map[string][]AlertLabels{
					"KubePodNotReady": {
						{
							"pod":       "test-pod",
							"namespace": "ns",
						},
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "get pod definitions with empty prefix falls back to single pod from alert",
			args: args{
				ctx: context.TODO(),
				params: GatherPodDefinitionParams{
					AlertName: "KubePodNotReady",
					PodPrefix: "", // Empty prefix should use old behavior
				},
				coreClient: func(ctx context.Context) v1.CoreV1Interface {
					coreClient := kubefake.NewClientset().CoreV1()
					// Create multiple pods but only one should be returned (from alert labels)
					for _, podName := range []string{"test-pod", "other-pod", "another-pod"} {
						_, err := coreClient.Pods("ns").Create(ctx, &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: "ns"},
							Status: corev1.PodStatus{
								Phase: corev1.PodPending,
							},
							Spec: corev1.PodSpec{},
						}, metav1.CreateOptions{})
						if err != nil {
							t.Fatalf("unable to create fake pod %s: %v", podName, err)
						}
					}
					return coreClient
				},
			},
			fields: fields{
				firingAlerts: map[string][]AlertLabels{
					"KubePodNotReady": {{"pod": "test-pod", "namespace": "ns"}},
				},
			},
			wantLen: 1, // Should only get the one pod from alert labels
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := &Gatherer{firingAlerts: tt.fields.firingAlerts}

			coreClient := tt.args.coreClient(tt.args.ctx)
			got, gotErr := g.gatherPodDefinition(tt.args.ctx, tt.args.params, coreClient)

			assert.Len(t, got, tt.wantLen)
			if tt.wantErr {
				assert.Len(t, gotErr, 1)
			} else {
				assert.Len(t, gotErr, 0)
			}
		})
	}
}

func Test_filterPodsByPrefix(t *testing.T) {
	tests := []struct {
		name      string
		podList   *corev1.PodList
		prefix    string
		wantLen   int
		wantNames []string
	}{
		{
			name: "filter pods with matching prefix",
			podList: &corev1.PodList{
				Items: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-0"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-mon-a"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "other-pod"}},
				},
			},
			prefix:    "rook-ceph-osd",
			wantLen:   3,
			wantNames: []string{"rook-ceph-osd-0", "rook-ceph-osd-1", "rook-ceph-osd-2"},
		},
		{
			name: "no pods match prefix",
			podList: &corev1.PodList{
				Items: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-a"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-b"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-c"}},
				},
			},
			prefix:    "non-existent",
			wantLen:   0,
			wantNames: []string{},
		},
		{
			name:      "empty pod list",
			podList:   &corev1.PodList{Items: []corev1.Pod{}},
			prefix:    "any-prefix",
			wantLen:   0,
			wantNames: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// When
			test := filterPodsByPrefix(tt.podList, tt.prefix)

			// Assert
			assert.Len(t, test, tt.wantLen)

			if tt.wantLen > 0 {
				gotNames := make([]string, len(test))
				for i, pod := range test {
					gotNames[i] = pod.Name
				}
				assert.ElementsMatch(t, tt.wantNames, gotNames)
			}
		})
	}
}
