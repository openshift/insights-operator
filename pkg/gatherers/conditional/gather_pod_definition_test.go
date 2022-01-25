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
					coreClient := kubefake.NewSimpleClientset().CoreV1()
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
	}
	for _, tt := range tests {
		tt := tt
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
