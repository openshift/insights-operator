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

func Test_loadRuntimeExtractorService(t *testing.T) {
	svc, err := loadRuntimeExtractorService()

	assert.NoError(t, err)
	assert.NotNil(t, svc)
	assert.Equal(t, serviceName, svc.Name)
	assert.Equal(t, daemonSetNamespace, svc.Namespace)
	assert.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, "https", svc.Spec.Ports[0].Name)
}

func Test_applyService(t *testing.T) {
	tests := []struct {
		name      string
		mockError error
		wantErr   bool
	}{
		{
			name:    "successfully applies service",
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
				coreClient.PrependReactor("create", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(nil, coreClient.CoreV1(), recorder)

			svc, err := rm.applyService(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, svc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, svc)
				assert.Equal(t, serviceName, svc.Name)
			}
		})
	}
}

func Test_deleteService(t *testing.T) {
	tests := []struct {
		name        string
		existingSvc *corev1.Service
		mockError   error
		wantErr     bool
	}{
		{
			name: "successfully deletes existing service",
			existingSvc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: daemonSetNamespace,
				},
			},
			wantErr: false,
		},
		{
			name:        "service not found - no error",
			existingSvc: nil,
			wantErr:     false,
		},
		{
			name: "delete error",
			existingSvc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
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
			if tt.existingSvc != nil {
				coreClient = fake.NewSimpleClientset(tt.existingSvc)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("delete", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(nil, coreClient.CoreV1(), recorder)

			err := rm.deleteService(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_serviceExists(t *testing.T) {
	tests := []struct {
		name        string
		existingSvc *corev1.Service
		mockError   error
		want        bool
	}{
		{
			name: "service exists",
			existingSvc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: daemonSetNamespace,
				},
			},
			want: true,
		},
		{
			name:        "service not found",
			existingSvc: nil,
			want:        false,
		},
		{
			name: "get error returns false",
			existingSvc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
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
			if tt.existingSvc != nil {
				coreClient = fake.NewSimpleClientset(tt.existingSvc)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("get", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(nil, coreClient.CoreV1(), recorder)

			got := rm.serviceExists(context.Background())

			assert.Equal(t, tt.want, got)
		})
	}
}
