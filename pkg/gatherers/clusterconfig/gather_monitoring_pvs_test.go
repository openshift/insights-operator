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
	testCases := []struct {
		name               string
		pvc                *corev1.PersistentVolumeClaim
		pv                 *corev1.PersistentVolume
		assertErrorsNumber int
		assertRecordNumber int
		assertRecord       bool
		assertError        bool
	}{
		{
			name: "Existent Persistent Volume within the namespace is gathered",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "mockName", Namespace: "mockNamespace"},
				Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "test"},
			},
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Status:     corev1.PersistentVolumeStatus{Phase: "Available"},
				Spec: corev1.PersistentVolumeSpec{
					PersistentVolumeSource: corev1.PersistentVolumeSource{},
				},
			},
			assertErrorsNumber: 0,
			assertRecordNumber: 1,
			assertRecord:       true,
		},
		{
			name: "Non-existent Persistent Volume within the namespace throws an error",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "mockName", Namespace: "mockNamespace"},
				Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "test"},
			},
			pv:                 &corev1.PersistentVolume{},
			assertErrorsNumber: 1,
			assertRecordNumber: 0,
			assertError:        true,
		},
		{
			name:               "Non-existent Persistent Volume Claim does not throw any error",
			pvc:                &corev1.PersistentVolumeClaim{},
			pv:                 &corev1.PersistentVolume{},
			assertErrorsNumber: 0,
			assertRecordNumber: 0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			coreclient := kubefake.NewSimpleClientset([]runtime.Object{testCase.pvc, testCase.pv}...)

			// When
			records, errors := gatherPVsByNamespace(context.TODO(), coreclient.CoreV1(), "mockNamespace")

			// Assert
			assert.Len(t, records, testCase.assertRecordNumber)
			if testCase.assertRecord {
				assert.Equal(t, "config/persistentvolumes/test", records[0].Name)
			}

			assert.Len(t, errors, testCase.assertErrorsNumber)
			if testCase.assertError {
				assert.ErrorContains(t, errors[0], "not found")
			}
		})
	}
}
