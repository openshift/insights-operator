package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
)

func Test_loadRuntimeExtractorConfigMap(t *testing.T) {
	cm, err := loadRuntimeExtractorConfigMap()

	assert.NoError(t, err)
	assert.NotNil(t, cm)
	assert.Equal(t, configMapName, cm.Name)
	assert.Equal(t, daemonSetNamespace, cm.Namespace)
	assert.Contains(t, cm.Data, "config.yaml")
}

func Test_applyConfigMap(t *testing.T) {
	tests := []struct {
		name      string
		mockError error
		wantErr   bool
	}{
		{
			name:    "successfully applies configmap",
			wantErr: false,
		},
		{
			name:      "apply error",
			mockError: apierrors.NewInternalError(assert.AnError),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreClient := fake.NewSimpleClientset()

			if tt.mockError != nil {
				coreClient.PrependReactor("create", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(nil, coreClient.CoreV1(), recorder)

			cm, err := rm.applyConfigMap(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cm)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cm)
				assert.Equal(t, configMapName, cm.Name)
			}
		})
	}
}

func Test_deleteConfigMap(t *testing.T) {
	tests := []struct {
		name       string
		existingCM *corev1.ConfigMap
		mockError  error
		wantErr    bool
	}{
		{
			name: "successfully deletes existing configmap",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: daemonSetNamespace,
				},
			},
			wantErr: false,
		},
		{
			name:       "configmap not found - no error",
			existingCM: nil,
			wantErr:    false,
		},
		{
			name: "delete error",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: daemonSetNamespace,
				},
			},
			mockError: apierrors.NewInternalError(assert.AnError),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var coreClient *fake.Clientset
			if tt.existingCM != nil {
				coreClient = fake.NewSimpleClientset(tt.existingCM)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("delete", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(nil, coreClient.CoreV1(), recorder)

			err := rm.deleteConfigMap(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_configMapExists(t *testing.T) {
	tests := []struct {
		name       string
		existingCM *corev1.ConfigMap
		mockError  error
		want       bool
	}{
		{
			name: "configmap exists",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: daemonSetNamespace,
				},
			},
			want: true,
		},
		{
			name:       "configmap not found",
			existingCM: nil,
			want:       false,
		},
		{
			name: "get error returns false",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: daemonSetNamespace,
				},
			},
			mockError: apierrors.NewServiceUnavailable("service unavailable"),
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var coreClient *fake.Clientset
			if tt.existingCM != nil {
				coreClient = fake.NewSimpleClientset(tt.existingCM)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("get", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(nil, coreClient.CoreV1(), recorder)

			got := rm.configMapExists(context.Background())

			assert.Equal(t, tt.want, got)
		})
	}
}
