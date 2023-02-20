package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_GatherMonitoring(t *testing.T) {

	t.Run("Existent Persistent Volume within the namespace is gathered", func(t *testing.T) {
		// Given
		mockPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "testName", Namespace: "testNamespace"},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "mockPVName"},
		}

		mockPV := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "mockPVName"},
			Status:     corev1.PersistentVolumeStatus{Phase: "Available"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{},
				StorageClassName:       "testStorageClass",
			},
		}
		coreclient := kubefake.NewSimpleClientset([]runtime.Object{mockPVC, mockPV}...)

		// When
		records, errors := gatherPVsByNamespace(context.TODO(), coreclient.CoreV1(), "testNamespace")

		// Assert
		assert.Len(t, errors, 0)
		assert.Len(t, records, 1)
		assert.Equal(t, "config/pod/testNamespace/mockPVName", records[0].Name)
	})
}
