package clusterconfig

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_GatherMonitoring(t *testing.T) {

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	ctx := context.Background()

	coreClient.Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
	}, metav1.CreateOptions{})

	claim, err := coreClient.PersistentVolumeClaims("test").Create(
		ctx,
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "amanda",
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("claim: %v\n", claim)

	volume, err := coreClient.PersistentVolumes().Create(
		ctx,
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Status: corev1.PersistentVolumeStatus{
				Phase: "Available",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("volume: %v\n", volume)

	list, err := coreClient.PersistentVolumeClaims("test").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("list: %v\n", list)

	coreClient.PersistentVolumes().Get(ctx, "", metav1.ListOptions{})
}
