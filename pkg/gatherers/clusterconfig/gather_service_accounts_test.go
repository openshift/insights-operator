package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_ServiceAccounts_Gather(t *testing.T) {
	tests := []struct {
		name            string
		namespaces      []string
		serviceAccounts []*corev1.ServiceAccount
		exp             string
	}{
		{
			name:       "one account",
			namespaces: []string{"default"},
			serviceAccounts: []*corev1.ServiceAccount{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-storage-operator",
					Namespace: "default",
				},
				Secrets: []corev1.ObjectReference{{}},
			}},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":1,"namespaces":{"default":[{"name":"local-storage-operator","secrets":1}]}}}`,
		},
		{
			name:       "multiple accounts",
			namespaces: []string{"openshift", "openshift-apiserver"},
			serviceAccounts: []*corev1.ServiceAccount{
				{
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
				},
			},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":2,"namespaces":{"openshift":[{"name":"deployer","secrets":1}],"openshift-apiserver":[{"name":"openshift-apiserver-sa","secrets":1}]}}}`, // nolint: lll
		},
		{
			name:       "multiple accounts on the same namespace",
			namespaces: []string{"openshift"},
			serviceAccounts: []*corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployer",
						Namespace: "openshift",
					},
					Secrets: []corev1.ObjectReference{{}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "builder",
						Namespace: "openshift",
					},
					Secrets: []corev1.ObjectReference{{}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "openshift",
					},
					Secrets: []corev1.ObjectReference{{}},
				},
			},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":3,"namespaces":{"openshift":[{"name":"builder","secrets":1},{"name":"default","secrets":1},{"name":"deployer","secrets":1}]}}}`, // nolint: lll
		},
		{
			name:       "multiple secrets",
			namespaces: []string{"default"},
			serviceAccounts: []*corev1.ServiceAccount{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-storage-operator",
					Namespace: "default",
				},
				Secrets: []corev1.ObjectReference{
					{
						Name: "secret1",
					},
					{
						Name: "secret2",
					},
				},
			}},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":1,"namespaces":{"default":[{"name":"local-storage-operator","secrets":2}]}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			coreClient := kubefake.NewSimpleClientset()
			for _, namespace := range test.namespaces {
				_, err := coreClient.CoreV1().Namespaces().Create(
					context.Background(),
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{},
				)
				assert.NoError(t, err)
			}
			for _, d := range test.serviceAccounts {
				_, err := coreClient.CoreV1().ServiceAccounts(d.Namespace).
					Create(context.Background(), d, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			serviceAccounts, errs := gatherServiceAccounts(context.Background(), coreClient.CoreV1())
			assert.Emptyf(t, errs, "Unexpected errors: %#v", errs)
			bts, err := serviceAccounts[0].Item.Marshal()
			assert.NoError(t, err)
			assert.EqualValues(t, test.exp, string(bts))
		})
	}
}
