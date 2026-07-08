package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_GatherRevisionedObjectCounts(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		configMaps []*corev1.ConfigMap
		secrets    []*corev1.Secret
		expected   string
	}{
		{
			name:       "objects with revision-status owner are counted",
			namespaces: []string{"openshift-kube-apiserver"},
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-1",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-2",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-2"},
						},
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "encryption-config-1",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-1"},
						},
					},
				},
			},
			expected: `{"openshift-kube-apiserver":{"configmaps":{"config":2},"secrets":{"encryption-config":1}}}`,
		},
		{
			name:       "objects without ownerReferences are skipped",
			namespaces: []string{"openshift-kube-apiserver"},
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-1",
						Namespace: "openshift-kube-apiserver",
					},
				},
			},
			secrets:  []*corev1.Secret{},
			expected: `{}`,
		},
		{
			name:       "objects with non-revision-status owners are skipped",
			namespaces: []string{"openshift-kube-apiserver"},
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-1",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "some-other-owner"},
						},
					},
				},
			},
			secrets:  []*corev1.Secret{},
			expected: `{}`,
		},
		{
			name:       "multiple revisions of same base name",
			namespaces: []string{"openshift-kube-apiserver"},
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "encryption-config-590",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-590"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "encryption-config-591",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-591"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd-client-608",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-608"},
						},
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "localhost-recovery-client-token-607",
						Namespace: "openshift-kube-apiserver",
						OwnerReferences: []metav1.OwnerReference{
							{Name: "revision-status-607"},
						},
					},
				},
			},
			expected: `{"openshift-kube-apiserver":` +
				`{"configmaps":{"encryption-config":2,"etcd-client":1},"secrets":{"localhost-recovery-client-token":1}}}`,
		},
		{
			name:       "empty namespace",
			namespaces: []string{"openshift-kube-apiserver"},
			configMaps: []*corev1.ConfigMap{},
			secrets:    []*corev1.Secret{},
			expected:   `{}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			coreClient := kubefake.NewClientset()

			// Create namespaces
			for _, namespace := range test.namespaces {
				_, err := coreClient.CoreV1().Namespaces().Create(ctx,
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}},
					metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			// Create ConfigMaps
			for _, cm := range test.configMaps {
				_, err := coreClient.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			// Create Secrets
			for _, secret := range test.secrets {
				_, err := coreClient.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			// Test gathering
			records, errs := gatherRevisionedObjectCounts(ctx, coreClient.CoreV1())
			assert.Empty(t, errs, "Unexpected errors: %#v", errs)
			assert.Len(t, records, 1, "Expected exactly 1 record")

			// Verify marshaled output
			bts, err := records[0].Item.Marshal()
			assert.NoError(t, err)
			assert.JSONEq(t, test.expected, string(bts))
		})
	}
}

func Test_ExtractBaseName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "name without revision suffix",
			input:    "some-config",
			expected: "some-config",
		},
		{
			name:     "name with multiple dashes and revision",
			input:    "kube-apiserver-pod-610",
			expected: "kube-apiserver-pod",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := extractBaseName(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func Test_HasRevisionStatusOwner(t *testing.T) {
	tests := []struct {
		name      string
		ownerRefs []metav1.OwnerReference
		expected  bool
	}{
		{
			name: "has revision-status owner among others",
			ownerRefs: []metav1.OwnerReference{
				{Name: "some-owner"},
				{Name: "revision-status-123"},
			},
			expected: true,
		},
		{
			name: "does not have revision-status owner",
			ownerRefs: []metav1.OwnerReference{
				{Name: "some-other-owner"},
			},
			expected: false,
		},
		{
			name:      "empty owner references",
			ownerRefs: []metav1.OwnerReference{},
			expected:  false,
		},
		{
			name:      "nil owner references",
			ownerRefs: nil,
			expected:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := hasRevisionStatusOwner(test.ownerRefs)
			assert.Equal(t, test.expected, result)
		})
	}
}
