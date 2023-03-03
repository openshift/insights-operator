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

func Test_GatherMonitoring_gather(t *testing.T) {
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
				ObjectMeta: metav1.ObjectMeta{Name: "mockName", Namespace: "openshift-monitoring"},
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
			name: "Existent Persistent Volume with unmatching prefix is not gathered",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "mockFail", Namespace: "openshift-monitoring"},
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
			assertRecordNumber: 0,
		},
		{
			name: "Non-existent Persistent Volume within the namespace throws an error",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "mockName", Namespace: "openshift-monitoring"},
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
			gatherer := MonitoringPVGatherer{client: coreclient.CoreV1()}

			// When
			records, errors := gatherer.gather(context.TODO(), "mockName")

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

func Test_GatherMonitoring_unmarshalDefaultPath(t *testing.T) {
	testCases := []struct {
		name     string
		yamlMock string
		expected string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "Trying to unmarshal expected data returns prometheus name",
			yamlMock: "prometheusK8s:\n  volumeClaimTemplate:\n    metadata:\n      name: mock\n    spec:\n      storageClassName: local-storage\n      resources:\n        requests:\n          storage: 2Gi\n",
			expected: "mock",
			wantErr:  false,
		},
		{
			name:     "Trying to unmarshal unexpected data returns an error",
			yamlMock: "prometheusK8s:\n  volumeClaimTemplate:\n    spec:\n      storageClassName: local-storage\n      resources:\n        requests:\n          storage: 2Gi\n",
			wantErr:  true,
			errMsg:   "can't find prometheusK8s.volumeClaimTemplate.metadata.name",
		},
		{
			name:     "Trying to unmarshal malformed yaml returns an error",
			yamlMock: "prometheusK8s:\n  volumeClaimTemplate:  dd: dfs:\n",
			wantErr:  true,
			errMsg:   "error converting YAML to JSON",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			data := testCase.yamlMock
			coreclient := kubefake.NewSimpleClientset()
			gatherer := MonitoringPVGatherer{client: coreclient.CoreV1()}

			// When
			test, err := gatherer.unmarshalDefaultPath(data)

			// Assert
			if testCase.wantErr {
				assert.Error(t, err)
				assert.ErrorContains(t, err, testCase.errMsg)

			} else {
				assert.Equal(t, testCase.expected, test)
				assert.NoError(t, err)
			}

		})
	}
}

func Test_GatherMonitoring_getDefaultPrometheusName(t *testing.T) {
	testCases := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "ConfigMap with valid data returns prometheus name",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-monitoring-config", Namespace: "openshift-monitoring"},
				Data: map[string]string{
					"config.yaml": "prometheusK8s:\n  volumeClaimTemplate:\n    metadata:\n      name: mock\n    spec:\n      storageClassName: local-storage\n      resources:\n        requests:\n          storage: 2Gi\n",
				},
			},
			expected: "mock",
		},
		{
			name:    "No cluster-monitoring-config configmap is defined returns an error",
			cm:      &corev1.ConfigMap{},
			wantErr: true,
			errMsg:  "configmaps \"cluster-monitoring-config\" not found",
		},
		{
			name: "ConfigMap having not config.yaml data entry returns an error",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-monitoring-config", Namespace: "openshift-monitoring"},
			},
			wantErr: true,
			errMsg:  "no config.yaml data on cluster-monitoring-config ConfigMap",
		},
		{
			name: "ConfigMap with unvalid data returns an error",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-monitoring-config", Namespace: "openshift-monitoring"},
				Data: map[string]string{
					"config.yaml": "otherdata:\n  ",
				},
			},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			coreclient := kubefake.NewSimpleClientset([]runtime.Object{testCase.cm}...)
			gatherer := MonitoringPVGatherer{client: coreclient.CoreV1()}

			// When
			test, err := gatherer.getDefaultPrometheusName(context.TODO())

			// Assert
			if testCase.wantErr {
				assert.Error(t, err)
				assert.ErrorContains(t, err, testCase.errMsg)

			} else {
				assert.Equal(t, testCase.expected, test)
				assert.NoError(t, err)
			}

		})
	}
}
