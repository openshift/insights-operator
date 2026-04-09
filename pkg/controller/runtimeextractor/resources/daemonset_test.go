package resources

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
)

func Test_loadRuntimeExtractorDaemonSet(t *testing.T) {
	ds, err := loadRuntimeExtractorDaemonSet()

	assert.NoError(t, err)
	assert.NotNil(t, ds)
	assert.Equal(t, daemonSetName, ds.Name)
	assert.Equal(t, daemonSetNamespace, ds.Namespace)
	assert.Len(t, ds.Spec.Template.Spec.Containers, 3)
}

func Test_applyDaemonSet(t *testing.T) {
	tests := []struct {
		name      string
		mockError error
		wantErr   bool
	}{
		{
			name:    "successfully applies daemonset",
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
				coreClient.PrependReactor("create", "daemonsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(coreClient.AppsV1(), coreClient.CoreV1(), recorder)

			ds, err := rm.applyDaemonSet(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, ds)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ds)
				assert.Equal(t, daemonSetName, ds.Name)
			}
		})
	}
}

func Test_updateContainerImages(t *testing.T) {
	// Set environment variables for this test
	os.Setenv(extractorImageEnv, "quay.io/openshift/runtime-extractor:v1.0.0")
	os.Setenv(exporterImageEnv, "quay.io/openshift/runtime-exporter:v1.0.0")
	os.Setenv(proxyImageEnv, "quay.io/openshift/proxy:v1.0.0")
	defer func() {
		os.Unsetenv(extractorImageEnv)
		os.Unsetenv(exporterImageEnv)
		os.Unsetenv(proxyImageEnv)
	}()

	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "extractor", Image: "old-image"},
						{Name: "exporter", Image: "old-image"},
						{Name: "kube-rbac-proxy", Image: "old-image"},
					},
				},
			},
		},
	}

	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
	rm := NewResourceManager(nil, nil, recorder)

	rm.updateContainerImages(ds)

	// Verify images were updated from environment variables
	assert.NotEqual(t, "old-image", ds.Spec.Template.Spec.Containers[0].Image)
	assert.NotEqual(t, "old-image", ds.Spec.Template.Spec.Containers[1].Image)
	assert.NotEqual(t, "old-image", ds.Spec.Template.Spec.Containers[2].Image)
}

func Test_deleteDaemonSet(t *testing.T) {
	tests := []struct {
		name       string
		existingDS *appsv1.DaemonSet
		mockError  error
		wantErr    bool
	}{
		{
			name: "successfully deletes existing daemonset",
			existingDS: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      daemonSetName,
					Namespace: daemonSetNamespace,
				},
			},
			wantErr: false,
		},
		{
			name:       "daemonset not found - no error",
			existingDS: nil,
			wantErr:    false,
		},
		{
			name: "delete error",
			existingDS: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      daemonSetName,
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
			if tt.existingDS != nil {
				coreClient = fake.NewSimpleClientset(tt.existingDS)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("delete", "daemonsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(coreClient.AppsV1(), coreClient.CoreV1(), recorder)

			err := rm.deleteDaemonSet(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_getDaemonSet(t *testing.T) {
	tests := []struct {
		name       string
		existingDS *appsv1.DaemonSet
		mockError  error
		wantErr    bool
	}{
		{
			name: "successfully gets daemonset",
			existingDS: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      daemonSetName,
					Namespace: daemonSetNamespace,
				},
			},
			wantErr: false,
		},
		{
			name:       "daemonset not found",
			existingDS: nil,
			wantErr:    true,
		},
		{
			name: "get error",
			existingDS: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      daemonSetName,
					Namespace: daemonSetNamespace,
				},
			},
			mockError: apierrors.NewServiceUnavailable("service unavailable"),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var coreClient *fake.Clientset
			if tt.existingDS != nil {
				coreClient = fake.NewSimpleClientset(tt.existingDS)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("get", "daemonsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(coreClient.AppsV1(), coreClient.CoreV1(), recorder)

			ds, err := rm.getDaemonSet(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ds)
				assert.Equal(t, daemonSetName, ds.Name)
			}
		})
	}
}

func Test_daemonSetExists(t *testing.T) {
	tests := []struct {
		name       string
		existingDS *appsv1.DaemonSet
		mockError  error
		want       bool
	}{
		{
			name: "daemonset exists",
			existingDS: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      daemonSetName,
					Namespace: daemonSetNamespace,
				},
			},
			want: true,
		},
		{
			name:       "daemonset not found",
			existingDS: nil,
			want:       false,
		},
		{
			name: "get error returns false",
			existingDS: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      daemonSetName,
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
			if tt.existingDS != nil {
				coreClient = fake.NewSimpleClientset(tt.existingDS)
			} else {
				coreClient = fake.NewSimpleClientset()
			}

			if tt.mockError != nil {
				coreClient.PrependReactor("get", "daemonsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.mockError
				})
			}

			recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
			rm := NewResourceManager(coreClient.AppsV1(), coreClient.CoreV1(), recorder)

			got := rm.daemonSetExists(context.Background())

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_loadImagesFromEnvs(t *testing.T) {
	tests := []struct {
		name            string
		extractorImage  string
		exporterImage   string
		proxyImage      string
		wantExtractor   string
		wantExporter    string
		wantProxy       string
		setExtractorEnv bool
		setExporterEnv  bool
		setProxyEnv     bool
	}{
		{
			name:            "all environment variables set",
			extractorImage:  "quay.io/openshift/runtime-extractor:v1.2.3",
			exporterImage:   "quay.io/openshift/runtime-exporter:v1.2.3",
			proxyImage:      "quay.io/openshift/kube-rbac-proxy:v1.2.3",
			wantExtractor:   "quay.io/openshift/runtime-extractor:v1.2.3",
			wantExporter:    "quay.io/openshift/runtime-exporter:v1.2.3",
			wantProxy:       "quay.io/openshift/kube-rbac-proxy:v1.2.3",
			setExtractorEnv: true,
			setExporterEnv:  true,
			setProxyEnv:     true,
		},
		{
			name:            "missing extractor environment variable uses default",
			exporterImage:   "quay.io/openshift/runtime-exporter:v1.0.0",
			proxyImage:      "quay.io/openshift/kube-rbac-proxy:v1.0.0",
			wantExtractor:   extractorDefaultImage,
			wantExporter:    "quay.io/openshift/runtime-exporter:v1.0.0",
			wantProxy:       "quay.io/openshift/kube-rbac-proxy:v1.0.0",
			setExtractorEnv: false,
			setExporterEnv:  true,
			setProxyEnv:     true,
		},
		{
			name:            "all environment variables empty use defaults",
			wantExtractor:   extractorDefaultImage,
			wantExporter:    exporterDefaultImage,
			wantProxy:       proxyDefaultImage,
			setExtractorEnv: false,
			setExporterEnv:  false,
			setProxyEnv:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean environment
			os.Unsetenv(extractorImageEnv)
			os.Unsetenv(exporterImageEnv)
			os.Unsetenv(proxyImageEnv)

			// Set environment variables as needed
			if tt.setExtractorEnv {
				os.Setenv(extractorImageEnv, tt.extractorImage)
			}
			if tt.setExporterEnv {
				os.Setenv(exporterImageEnv, tt.exporterImage)
			}
			if tt.setProxyEnv {
				os.Setenv(proxyImageEnv, tt.proxyImage)
			}

			defer func() {
				os.Unsetenv(extractorImageEnv)
				os.Unsetenv(exporterImageEnv)
				os.Unsetenv(proxyImageEnv)
			}()

			gotExtractor, gotExporter, gotProxy := loadImagesFromEnvs()

			assert.Equal(t, tt.wantExtractor, gotExtractor)
			assert.Equal(t, tt.wantExporter, gotExporter)
			assert.Equal(t, tt.wantProxy, gotProxy)
		})
	}
}
