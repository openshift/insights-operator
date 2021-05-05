package clusterconfig

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_ServiceAccounts_Gather(t *testing.T) {
	tests := []struct {
		name string
		data []*corev1.ServiceAccount
		exp  string
	}{
		{
			name: "one account",
			data: []*corev1.ServiceAccount{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-storage-operator",
					Namespace: "default",
				},
				Secrets: []corev1.ObjectReference{{}},
			}},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":1,"namespaces":{"default":{"name":"local-storage-operator","secrets":1}}}}`,
		},
		{
			name: "multiple accounts",
			data: []*corev1.ServiceAccount{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployer",
					Namespace: "openshift",
				},
				Secrets: []corev1.ObjectReference{{}},
			},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openshift-apiserver-sa",
						Namespace: "openshift-apiserver",
					},
					Secrets: []corev1.ObjectReference{{}},
				}},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":2,"namespaces":{"openshift":{"name":"deployer","secrets":1},"openshift-apiserver":{"name":"openshift-apiserver-sa","secrets":1}}}}`, // nolint: lll
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			coreClient := kubefake.NewSimpleClientset()
			for _, d := range test.data {
				_, err := coreClient.CoreV1().Namespaces().Create(
					context.Background(),
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: d.Namespace}}, metav1.CreateOptions{},
				)
				if err != nil {
					t.Fatalf("unable to create fake ns %s", err)
				}
				_, err = coreClient.CoreV1().ServiceAccounts(d.Namespace).
					Create(context.Background(), d, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("unable to create fake service account %s", err)
				}
			}
			sa, errs := gatherServiceAccounts(context.Background(), coreClient.CoreV1())
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %#v", errs)
				return
			}
			bts, err := sa[0].Item.Marshal(context.Background())
			if err != nil {
				t.Fatalf("error marshaling %s", err)
			}
			s := string(bts)
			if test.exp != s {
				t.Fatalf("serviceaccount test failed. expected: %s got: %s", test.exp, s)
			}
		})
	}
}
